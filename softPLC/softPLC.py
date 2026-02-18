# softPLC  - A Click PLC emulator
#   Console:  http://testbed.fsbhoa.com:8020/
#             http://testbed.fsbhoa.com:8021/
#   Configuration file: plc_config_lodge.json or plc_config_cabana.json
#   python3 softPLC.py plc_config_lodge.json
#
import re
import os
import asyncio
import logging
import json
import sys
import operator
import time
import threading
from datetime import datetime
from flask import Flask, render_template_string, redirect, request
from pymodbus.server import StartAsyncTcpServer
from pymodbus.device import ModbusDeviceIdentification
from pymodbus.datastore import ModbusSequentialDataBlock, ModbusSlaveContext, ModbusServerContext

class LogicResetException(Exception): pass

# Setup logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger("SoftPLC")

# Mute the Flask/Werkzeug "GET /" requests
log = logging.getLogger('werkzeug')
log.setLevel(logging.ERROR)

# --- 1. MEMORY MANAGER ---
class MemoryManager:
    def __init__(self, context, slave_id=0x01, config=None):
        self.step_mode = True
        self.step_trigger = False
        self.rung_id = 0
        self.last_rung = "Waiting for first scan..."
        self.next_rung = "Waiting..."
        self.context = context
        self.timer_accumulators = {}
        self.first_scan_done = False
        self.slave_id = slave_id
        self.slow_mo_active = False
        self.scan_trigger = False
        self.is_paused = False
        self.logic_reset_requested = False
        self.breakpoint_expr = ""
        self.breakpoint_in_progress = False
        self.syntax_errors = []
        self.sticky_map = {}
        self.needs_save = False
        # CLICK to Modbus Offset Map (0-based)
        self.map = {'X': 9000, 'Y': 8192, 'C': 16384, 'DS': 0, 'TD': 45056, 'DH': 10000, 'YD': 8192}
        # Initialize memory immediately on startup
        if config:
            self._build_sticky_map(config)
            self.clear_volatile_memory(config)

            # Look for 'startup_running' in config. Default to False (Step Mode) if not found.
            val = config.get('startup_running', False)
            if str(val).lower() == 'true':
                self.step_mode = False
                logger.info("MEM: Config set to STARTUP_RUNNING. Initializing in Free-Run.")
            else:
                self.step_mode = True
                logger.info("MEM: Initializing in STEP_MODE (Paused).")

    def _build_sticky_map(self, config):
        """Processes config once to create a high-speed lookup for retentive ranges."""
        ranges = config.get('retentive_ranges', [])
        self.sticky_map = {}
        
        for r in ranges:
            # Extract prefix (e.g., 'DS') and start/end numbers
            prefix = ''.join([c for c in r['start'] if c.isalpha()])
            try:
                start_num = int(''.join([c for c in r['start'] if c.isdigit()]))
                end_num = int(''.join([c for c in r['end'] if c.isdigit()]))
                
                if prefix not in self.sticky_map:
                    self.sticky_map[prefix] = []
                self.sticky_map[prefix].append((start_num, end_num))
            except ValueError:
                logger.error(f"MEM: Invalid retentive range in config: {r}")
        
        logger.info(f"MEM: Sticky map initialized for: {list(self.sticky_map.keys())}")


    def clear_volatile_memory(self, config):
        """Standardizes all registers to 0 except for defined retentive ranges."""
        # Ensure map is current if config changed
        self._build_sticky_map(config) 
        
        persistence_file = config.get('persistence_file', 'retentive_memory.json')
        MAX_CLEAR_RANGE = 2000
        prefixes = ['C', 'DS', 'DH', 'Y', 'T', 'CT']

        for prefix in prefixes:
            m_type, base_addr = self._resolve_label(f"{prefix}1")
            if m_type is None:
                continue

            for i in range(MAX_CLEAR_RANGE):
                reg_num = i + 1
                label = f"{prefix}{reg_num}"
                
                # Use the class helper
                if not self._is_retentive(f"{prefix}{reg_num}"):
                    addr = base_addr + i
                    self.context[self.slave_id].setValues(m_type, addr, [0])

        # Fill the holes with the saved data
        self.load_from_disk(persistence_file)
        self.first_scan_done = False
        self.needs_save = False



    def load_from_disk(self, filename):
        if os.path.exists(filename):
            try:
                with open(filename, 'r') as f:
                    stored_data = json.load(f)
                    for addr_label, val in stored_data.items():
                        self.write(addr_label, val)
                #logger.info(f"Persistence: Loaded {len(stored_data)} registers from {filename}")
            except Exception as e:
                logger.error(f"Persistence: Load failed: {e}")
        self.needs_save = False

    def _is_retentive(self, label):
        """High-speed check using the class-level sticky_map."""
        prefix = ''.join([c for c in label if c.isalpha()])
        try:
            num = int(''.join([c for c in label if c.isdigit()]))
        except (ValueError, IndexError):
            return False

        if prefix in self.sticky_map:
            for start, end in self.sticky_map[prefix]:
                if start <= num <= end:
                    return True
        return False

    def save_to_disk(self, config):
        ranges = config.get('retentive_ranges', [])
        filename = config.get('persistence_file', 'retentive_memory.json')
        data_to_save = {}

        for r in ranges:
            # Extract prefix and numeric range
            prefix = ''.join([c for c in r['start'] if c.isalpha()])
            start_num = int(''.join([c for c in r['start'] if c.isdigit()]))
            end_num = int(''.join([c for c in r['end'] if c.isdigit()]))
        
            for i in range(start_num, end_num + 1):
                addr_label = f"{prefix}{i}"
                # Read current value and store in the dictionary
                data_to_save[addr_label] = self.read(addr_label)

        try:
            with open(filename, 'w') as f:
                json.dump(data_to_save, f, indent=4)
            #logger.info(f"Persistence: Successfully saved {len(data_to_save)} registers to {filename}")
        except Exception as e:
            logger.error(f"Persistence: Save failed: {e}")


    def _resolve_label(self, label):
        match = re.match(r"([A-Z]+)(\d+)", label)
        if not match: return None, None
        prefix, offset = match.groups()
        
        # 1. Determine Modbus Type (1=Coil/Bit, 3=Holding Reg/Word)
        m_type = 1 if prefix in ['X', 'Y', 'C'] else 3
        
        # 2. Use the specific base offsets from your Go Service Map
        # CLICK is 1-based, Modbus is 0-based, so we subtract 1 from offset
        base_addr = self.map.get(prefix, 0)
        final_addr = base_addr + (int(offset) - 1)
        
        return m_type, final_addr

    def read(self, label):
        if label.upper() == "ALWAYS":
            return True
        if label == "SC1": return True  # _Always_ON
        if label == "SC2":              # _1st_SCAN
            if not self.first_scan_done:
                return True
            return False
        if label == "SC3":              # _SCAN_Clock (Toggles every scan)
            self.scan_clock = getattr(self, 'scan_clock', False)
            self.scan_clock = not self.scan_clock
            return self.scan_clock
        
        m_type, addr = self._resolve_label(label)
        if m_type is None: return 0
        val = self.context[self.slave_id].getValues(m_type, addr, count=1)[0]
        return bool(val) if m_type == 1 else val

    def write(self, label, value):
        # --- Type Safety Check ---
        # Allow only ints, floats (which we'll cast), and bools
        if not isinstance(value, (int, float, bool)):
            err_msg = f"MEM CRITICAL: Attempted to write invalid type {type(value)} to {label} (Value: {value})"
            logger.error(err_msg)
            self.add_error(err_msg)
            return # Block the write

        if label.startswith('YD'):
            # Log the 16-bit pattern for your SSH window
            logger.info(f"*** WORD OUTPUT: {label} = {bin(value)} (Hex: {hex(value)})")
            
            # Resolve the base address (e.g., YD1 -> 8192)
            m_type, base_addr = self._resolve_label(label.replace('YD', 'Y'))
            
            # Faithfully update the 16 individual coils in the Modbus store
            bit_values = [(value >> i) & 1 for i in range(16)]
            self.context[self.slave_id].setValues(m_type, base_addr, bit_values)
        else:
            m_type, addr = self._resolve_label(label)
            if m_type:
                self.context[self.slave_id].setValues(m_type, addr, [int(value)])
        # Check if this address is in our 'sticky' list
        if self._is_retentive(label):
            self.needs_save = True

    def resolve_deref(self, arg):
        # Strict rule: only resolve if brackets exist
        match = re.search(r"([A-Z]+)\[([A-Z0-9]+)\]", arg)
        if match:
            prefix, ptr = match.groups()
            ptr_val = self.read(ptr)
            return f"{prefix}{ptr_val}"
        return arg

    def read_timer_acc(self, timer_id):
        """Returns the current milliseconds for a timer, default 0."""
        # This prevents the 'NoneType' error by ensuring a 0 is returned
        return self.timer_accumulators.get(timer_id, 0)

    def write_timer_acc(self, timer_id, ms_value):
        """Sets the current milliseconds for a timer."""
        self.timer_accumulators[timer_id] = ms_value

    def append_display(self, text):
        """Appends a line of diagnostic text to the current last_rung display."""
        if self.step_mode:
            # We use a non-breaking space and a cyan color to make assignments pop
            self.last_rung += f"<br>&nbsp;&nbsp;Rung {self.rung_id} <span style='color:#00ffff'>» {text}</span>"
            logger.info(f"Rung {self.rung_id} {text}")

    def add_error(self, msg):
        if msg not in self.syntax_errors:
            self.syntax_errors.append(msg)

    def clear_errors(self):
        self.syntax_errors = []

# --- 2. CLICK PARSER ---
class CLICKParser:
    def __init__(self, mem=None):
        self.mem = mem  # Now the parser can "speak" to the dashboard

    # Updated Parser logic to strip comments first
    def parse(self, filename):
        programs = {"MAIN": []}
        current_block = "MAIN"
        with open(filename, 'r') as f:
            # We process the whole file, but split it before the Memory Map
            raw_content = f.read().split("## Memory Map")[0]

        accumulated_rung = ""
        for line in raw_content.splitlines():
            # 1. Strip anything after //
            clean_line = line.split("//")[0].strip()
        
            # 2. Skip truly empty lines
            if not clean_line:
                continue

            # 3. Handle Subroutine and Rung detection
            if clean_line.startswith("## Subroutine"):
                self._flush_rung(programs, current_block, accumulated_rung)
                accumulated_rung = ""
                current_block = clean_line.split("Subroutine:")[1].strip().upper()
                programs[current_block] = []
            elif clean_line.startswith("Rung"):
                self._flush_rung(programs, current_block, accumulated_rung)
                accumulated_rung = clean_line
            else:
                # Append continuations (like parallel actions) to the current rung string
                accumulated_rung += " " + clean_line

        self._flush_rung(programs, current_block, accumulated_rung)
        return programs

    def _flush_rung(self, programs, block, text):
        if not text:
            return
            
        # Initialize the raw text storage for this block if it doesn't exist
        if "__raw__" not in programs:
            programs["__raw__"] = {}
        if block not in programs["__raw__"]:
            programs["__raw__"][block] = []

        # Find the Rung ID and the Logic Body
        match = re.match(r"Rung\s+(\d+):\s*(.*)", text)
        if match:
            rid, body = match.groups()
            if "->" in body:
                parts = [p.strip() for p in body.split("->", 1)]
                if len(parts) != 2:
                    logger.error(f"UNPACK ERROR: Expected 2 parts, found {len(parts)}. Rung: {text}")
                    return # Skip this rung and move to the next
                c_part, a_part = parts
                
                # Store the parsed data for execution
                programs[block].append({
                    "id": int(rid),
                    "conds": c_part,
                    "acts": self._parse_acts(a_part)
                })
                
                # Store the original text for the Dashboard diagnostic
                programs["__raw__"][block].append(text)
            else:
                # THIS IS THE WATCHDOG
                if text.strip():
                    logger.error(f"PARSER FAIL: Line looks like a Rung but failed regex: '{text}'")
                    self.mem.add_error(f"Syntax Error: {text[:30]}...")



    def _parse_acts(self, text):
        acts = []
        
        # 1. Strip comments first as requested
        # This removes everything from // to the end of the line
        clean_text = re.sub(r"//.*$", "", text).strip()
        
        # 2. Extract balanced parentheses blocks
        ## This regex looks for: (Instruction ARGS)
        ## It handles nested parentheses by looking for the outermost pairs
        #pattern = r"\((\w+)(?:\s+((?:[^\(\)]|\([^\(\)]*\))*))?\)"
        #raw_acts = re.findall(pattern, clean_text)
        
        #for inst, args in raw_acts:
        #    inst = inst.upper()
        #    args = args.strip()
        #    print(f"DEBUG PARSER: Found {inst} with args {args}")
        #    
        #    # Handle the specific case of (parallel) -> which is a marker, not an act
        #    if inst == "PARALLEL":
        #        continue
        #        
        #    acts.append({"inst": inst, "args": args})
        # 2. Balanced Parentheses Scanner
        stack = 0
        start_idx = -1
        
        for i, char in enumerate(clean_text):
            if char == '(':
                if stack == 0:
                    start_idx = i
                stack += 1
            elif char == ')':
                stack -= 1
                if stack == 0 and start_idx != -1:
                    # We found a complete outermost block: (INST ARGS)
                    content = clean_text[start_idx + 1:i].strip()
                    
                    # Split content into Instruction and Args
                    # This splits only on the first whitespace found
                    parts = content.split(None, 1)
                    inst = parts[0].upper()
                    args = parts[1] if len(parts) > 1 else ""
                    
                    if inst != "PARALLEL":
                        acts.append({"inst": inst, "args": args})
                    
                    start_idx = -1
        return acts
            
    def _report_error(self, reason):
        # Create a "Visual Pointer" error message
        # e.g., "Naked text in: [C1] C2 [C3] --> ^ near 'C2'"
        #context = full_text[max(0, pos-10):min(len(full_text), pos+10)]
        #error_msg = f"{reason} in logic: ...{context}... (at '{full_text[pos:pos+1]}' pos {pos})"
        
        # This writes to the console and the Dashboard-accessible memory
        logger.error(reason)
        self.mem.add_error(reason)

# --- 3. LOGIC ENGINE ---
class LogicEngine:
    def __init__(self, memory, programs):
        self.mem = memory
        self.programs = programs
        self.tmr_state = {} # Stores start times for TMR instructions

    def run_block(self, block_name):
        rungs = self.programs.get(block_name, [])
        raw_rungs = self.programs.get("__raw__", {}).get(block_name, [])
        ptr = 0
        loop_stack = []
        max_ops = 10000
        ops = 0

        while ptr < len(rungs):
            ops += 1
            if ops > max_ops:
                logger.error(f"SCAN HALTED: Infinite loop detected in {block_name}")
                break

            rung = rungs[ptr]

            # --- LANDMARK 1: Dashboard "NEXT" Update ---
            current_rung_text = raw_rungs[ptr] if ptr < len(raw_rungs) else "Unknown Rung"
            self.mem.rung_id = rung.get('id', 0)
            self.mem.next_rung = f"({block_name}) {current_rung_text}"

            # --- LANDMARK 2: Single Step Pause ---
            if self.mem.step_mode:
                # Determine if we should skip the pause because a 'Next Scan' is in progress
                is_start_of_main = (block_name == "MAIN" and ptr == 0)
                if not self.mem.scan_trigger or is_start_of_main:
                
                    self.mem.is_paused = True
                    self.mem.scan_trigger = False

                    # The actual wait loop
                    print(f"PAUSED: {block_name} Rung {rung.get('id')}")
                    while not self.mem.step_trigger:
                        # BREAK if scan_trigger was just pressed
                        if self.mem.scan_trigger: 
                            self.mem.last_rung = ""
                            break 
                        # If user clicks "Exit Step Mode", break out
                        if not self.mem.step_mode: 
                            break
                        # Check for Reset while waiting
                        if getattr(self.mem, 'logic_reset_requested', False):
                            shared_mem.last_rung = "<b>COLD BOOT</b>: Memory Map Initialized."
                            self.mem.is_paused = False
                            raise LogicResetException()
                        time.sleep(0.05)
                
                    self.mem.is_paused = False
                    self.mem.step_trigger = False

            jumped = False

            # --- LANDMARK 3: Condition Evaluation  ---
            cond_passed, details = self.eval_conds(rung['conds'])


            # --- LANDMARK 4: Dashboard "LAST" Update (after execution) ---
            if not self.mem.scan_trigger:
                status = "EXEC" if cond_passed else "SKIP"
            
                display_text = f"[{status}] ({block_name}) {current_rung_text}"
            
                # Append each condition with its status
                for text, success in details:
                    color = "#00ff41" if success else "#ff4500" # Green or Red-Orange
                    display_text += f"<br>   <span style='color:{color}'>↳ {text}</span>"
            
                self.mem.last_rung = display_text


            # --- LANDMARK 5: Execute Actions if Conditions Passed ---
            for act in rung['acts']:
                inst = act['inst']
                args = act['args']

                if inst == "FOR" and cond_passed:
                    # Clean the argument (remove comments/whitespace)
                    arg_val = args.split('//')[0].strip()
                    
                    # Check if the first character is a digit
                    if arg_val[0].isdigit():
                        count = int(arg_val)
                    else:
                        # Resolve the memory address (e.g., "DS99")
                        count = self.mem.read(arg_val)
    
                    # Safety: Clamp to 1 to prevent infinite loops or empty stacks
                    count = max(int(count), 1)
    
                    loop_stack.append({"start": ptr, "curr": 1, "max": count})
                    self.mem.append_display(f"[FOR {count}]")
                    
                elif inst == "NEXT" and cond_passed:
                    if loop_stack:
                        loop = loop_stack[-1]
                        if loop["curr"] < loop["max"]:
                            loop["curr"] += 1
                            ptr = loop["start"] + 1
                            jumped = True
                            self.mem.append_display(f"[NEXT]: Looping to {loop['curr']}/{loop['max']}")
                            break
                        else:
                            loop_stack.pop()
                            self.mem.append_display("[NEXT]: Loop Complete")
                                
                elif inst == "CALL" and cond_passed:
                        name = args.strip().upper()
                        self.mem.append_display(f"[CALL {name}]")
                        self.run_block(name)
                    
                elif inst == "RET" and cond_passed:
                        self.mem.append_display("[RET]")
                        return

                elif inst == "END":
                        self.mem.append_display("[END]")
                        return
                    
                self.execute_standard(act, cond_passed)



            if not jumped:
                ptr += 1

            # --- Checkpoint Intercept ---
            if self.mem.breakpoint_in_progress and self.mem.breakpoint_expr:
                # We call eval_cmp to check the condition
                passed, trace = self.eval_cmp(self.mem.breakpoint_expr)
    
                # If the comparison itself failed to parse, stop immediately
                if "Regex Fail" in trace:
                    logger.error(f"BREAKPOINT ERROR: Invalid Syntax '{self.mem.breakpoint_expr}'")
                    self.mem.breakpoint_in_progress = False
                    self.mem.step_mode = True
                elif passed:
                    logger.info(f"!!! BREAKPOINT HIT: {trace} !!!")
                    self.mem.breakpoint_in_progress = False
                    self.mem.step_mode = True


    def run_scan(self, slow_mo=False):
        # Update RTC
        now = datetime.now()
        self.mem.write("SD24", now.hour)
        self.mem.write("SD25", now.minute)
        self.mem.write("SD23", now.isoweekday() % 7 + 1)

        if slow_mo:
            logger.info(f"--- START SCAN: DS3={self.mem.read('DS3')} ---")
        
        # Run Main
        try:
            self.run_block("MAIN")
            self.mem.scan_trigger = False # Reset the "Next Scan" permission
            self.mem.first_scan_done = True # resets SC1
        except LogicResetException:
            # Reset caught! We clear the flag and log it.
            # The next time run_scan is called by the logic_loop, 
            # it will start at the top of MAIN.
            self.mem.logic_reset_requested = False
            logger.info("Logic Stack Collapsed: Restarting from MAIN.")
            self.mem.next_rung = "(MAIN) Rung 1: System Restarting..."
            return # Exit this scan immediately


    def extract_nested_blocks(self, s):
        """
        Scanner: Extracts top-level bracketed units while respecting nesting.
        Input: "[OR [C151] [CMP A!=B]] [C100]"
        Output: ["OR [C151] [CMP A!=B]", "C100"]
        """
        results = []
        count = 0
        start = 0
        for i, char in enumerate(s):
            if char == '[':
                if count == 0:
                    start = i + 1  # Start just after the opening bracket
                count += 1
            elif char == ']':
                count -= 1
                if count == 0:
                    results.append(s[start:i]) # Content inside the brackets
        return results

    def eval_conds(self, cond_str):
        details = []
        cond_str = cond_str.strip()

        # Handle the case where the string is NOT bracketed (Terminal Leaf)
        if not cond_str.startswith('['):
            success = False
            upper = cond_str.upper()
            if upper.startswith("CMP "):
                success, math_str = self.eval_cmp(cond_str[4:])
                details.append((f" CMP {math_str}={success}", success))
            elif upper.startswith("NOT "):
                label = cond_str[4:].strip()
                success = not self.mem.read(label)
                details.append((f" NOT ({label})={success}", success))
            else:
                success = self.mem.read(cond_str)
                details.append((f" {cond_str}={success}", success))
            return success, details

        # Otherwise, split into top-level blocks [A][B] (Implicit AND)
        blocks = self.extract_nested_blocks(cond_str)
        for block in blocks:
            success = False
            block_upper = block.upper().strip()

            if block_upper.startswith("OR "):
                branches = self.extract_nested_blocks(block[3:])
                branch_match = False
                details.append(("OR (", "header"))
                for b_str in branches:
                    b_res, b_details = self.eval_conds(b_str)
                    details.extend(b_details)
                    
                    if b_res:
                        branch_match = True
                        details.extend(b_details)
                        break 
                details.append((f") = {branch_match}", branch_match))
                success = branch_match

            elif block_upper.startswith("AND "):
                success, sub_details = self.eval_conds(block[4:])
                details.append(("AND (", "header"))
                details.extend(sub_details)
                details.append((f") = {success}", success))

            else:
                # Recurse to handle the content of the bracket
                success, sub_details = self.eval_conds(block)
                details.extend(sub_details)

            if not success:
                return False, details

        return True, details


    def eval_cmp(self, expr):
        try:
            # 1.  Regex: Allows Parentheses, *, +, and Spaces for Clock Math
            # Group 1: Left side (Supports DS30, (SD24 * 100) + SD25, etc.)
            # Group 2: Operator (==, !=, >, etc.)
            # Group 3: Right side
            match = re.search(r"(.+?)\s*([><=!]+)\s*(.+)", expr)
            if not match:
                logger.warning(f"CMP Regex Failed: '{expr}'")
                return False, f"Regex Fail: {expr}"

            v1_label, op, v2_label = match.groups()
            v1_label, v2_label = v1_label.strip(), v2_label.strip()

            # 2. Get actual values (handling the 'DS3' vs '24' logic)
            val1 = self.mem.read(v1_label) if not v1_label.isdigit() else int(v1_label)
            val2 = self.mem.read(v2_label) if not v2_label.isdigit() else int(v2_label)

            # 3. Force to Integers to avoid string-sorting errors (e.g., "9" > "10")
            n1 = int(val1 if val1 is not None else 0)
            n2 = int(val2 if val2 is not None else 0)


            # 4. Perform comparison
            ops = {
                '>': operator.gt, '<': operator.lt, '==': operator.eq, '=': operator.eq,
                '!=': operator.ne, '<>': operator.ne, '>=': operator.ge, '<=': operator.le
            }
        
            # Default to False if operator is weird
            result = ops.get(op, lambda x, y: False)(n1, n2)

            # 5. ALWAYS return a tuple (result, trace_string)
            trace = f"({v1_label}({n1}) {op} {v2_label}({n2}))"
        
            return result, trace

        except Exception as e:
            logger.error(f"Logic Engine CMP Error on '{expr}': {e}")
            return False, f"Error: {e}"

    def _resolve_constant(self, val_str):
        """Converts CLICK strings (K200, 10h) to Python integers."""
        val_str = val_str.strip()
        
        # 1. Handle Hex: 10h -> 16
        if val_str.lower().endswith('h'):
            return int(val_str[:-1], 16)
            
        # 2. Handle CLICK Decimal Constants: K200 -> 200
        if val_str.upper().startswith('K'):
            try:
                # Strip the K and convert the rest to int
                k_val = val_str[1:]
                if k_val.isdigit(): 
                    return int(k_val)
                else: 
                    return self.mem.read(k_val)
            except ValueError:
                # Fallback: if it's something like KDS1, read the register
                return self.mem.read(val_str[1:])
                
        # 3. Handle Raw Integers
        if val_str.isdigit() or (val_str.startswith('-') and val_str[1:].isdigit()):
            return int(val_str)
            
        # 4. Otherwise, treat as a register address (DS1, etc.)
        return self.mem.read(val_str)

    def execute_standard(self, act, cond_passed):
        inst, args = act['inst'], act['args']
        
        if cond_passed and  inst == "COPY":
            if "->" in args:
                parts = [p.strip() for p in args.split("->")]
                if len(parts) != 2:
                    logger.error(f"COPY Unpack Error in {args}. Got {len(parts)} parts.")
                    return # Skip this rung instead of crashing the loop
                src_str, dest_str = parts
                
                def resolve_indexed(label):
                    # Check for patterns like DS[DS20]
                    match = re.search(r'([A-Z]+)\s*\[\s*([A-Z]+\d+)\s*\]', label, re.IGNORECASE)
                    if match:
                        base_type, index_reg = match.groups()
                        index_val = self.mem.read(index_reg)
                        resolved = f"{base_type}{index_val}"
                        # LOUD DEBUG: See exactly what the index turns into
                        #logger.info(f"INDEX RESOLVED: {label} -> {resolved}")
                        return resolved
                    return label

                # 1. Resolve source and destination addresses
                actual_src_addr = resolve_indexed(src_str)
                actual_dest_addr = resolve_indexed(dest_str)

                # 2. Get the value from source
                val = self._resolve_constant(actual_src_addr)

                # 3. Write to destination
                self.mem.write(actual_dest_addr, val)
                
                if self.mem.step_mode:
                    s_addr = actual_src_addr
                    d_addr = actual_dest_addr
                    if src_str != actual_src_addr:
                        s_addr += f" (Indexed: {src_str})"
                    if dest_str != actual_dest_addr:
                        d_addr += f" (Indexed: {dest_str})"
                    self.mem.append_display(f"[COPY] {val} [{s_addr}] -> {d_addr}")
            else:
                logger.error(f"COPY Unpack Error in {args}. Missing ->")

        elif cond_passed and inst == "MATH":
            # Handles: DS30 = (SD24 * 100) + SD25
            if "=" in args:
                # Split target and formula (e.g., DS30 = (SD24 * 100) + SD25)
                parts = [p.strip() for p in args.split("=", 1)]
                if len(parts) != 2:
                    logger.error(f"MATH Unpack Error in {args}")
                    return
                target, expr = parts
                calc_expr = expr
                
                # 1. Translate CLICK Hex (10h) to Python Hex (0x10)
                # This regex looks for digits/A-F followed by 'h'
                calc_expr = re.sub(r'(\b[0-9A-Fa-f]+)h\b', r'0x\1', calc_expr)
                calc_expr = re.sub(r'\bK(\d+)\b', r'\1', calc_expr) # Strip K
                
                # 2. Translate CLICK LSH(val, shift) to Python (val << shift)
                calc_expr = re.sub(r'LSH\((.*?), (.*?)\)', r'((\1) << (\2))', calc_expr)

                # 3. # Translate CLICK MOD to Python %
                calc_expr = re.sub(r'MOD', '%', calc_expr, flags=re.IGNORECASE)

                # 4. Resolve Register Tokens (DS1, SD24, etc.)
                # We sort keys by length descending so DS10 isn't partially replaced by DS1
                tokens = re.findall(r'[A-Z]+\d+', calc_expr)
                for t in sorted(set(tokens), key=len, reverse=True):
                    val = self.mem.read(t)
                    val = val if val is not None else 0
                    calc_expr = calc_expr.replace(t, str(val))

                try:
                    # 5. Final Parenthesis Safety Check
                    open_c = calc_expr.count('(')
                    close_c = calc_expr.count(')')
                    if open_c > close_c:
                        calc_expr += ')' * (open_c - close_c)
                    elif close_c > open_c:
                        calc_expr = calc_expr.rstrip(')')
                    
                    # 6. Evaluate and Write Result
                    result = int(eval(calc_expr))
                    self.mem.write(target, result)
                    
                    # Step Mode Diagnostic
                    self.mem.append_display(f"[MATH] {target} = {result} (Source: {calc_expr.strip()})")
                        
                except Exception as e:
                    self.mem.append_display(f"MATH Error: {target} = {expr} (Evaluated: {calc_expr}) -> {e}")
            else:
                logger.error(f"MATH Error: {target} = {expr} Missing =")

        elif cond_passed and inst in ["SET", "RST"]:
            val = 1 if inst == "SET" else 0
            target_raw = args.strip()

            # Check if this is a range: "C1011 to C1012"
            if " to " in target_raw.lower():
                parts = re.split(r'\s+to\s+', target_raw, flags=re.IGNORECASE)
                if len(parts) == 2:
                    start_label = parts[0].strip()
                    end_label = parts[1].strip()

                    # Extract prefix and numbers
                    prefix = ''.join([c for c in start_label if c.isalpha()])
                    start_num = int(''.join([c for c in start_label if c.isdigit()]))
                    end_num = int(''.join([c for c in end_label if c.isdigit()]))

                    for i in range(start_num, end_num + 1):
                        self.mem.write(f"{prefix}{i}", val)
                    
                    if self.mem.step_mode:
                        self.mem.append_display(f"[{inst} RANGE] {prefix}{start_num}-{end_num} = {val}")
            else:
                # Standard single-bit logic
                self.mem.write(target_raw, val)
                if self.mem.step_mode:
                    self.mem.append_display(f"[{inst}] {target_raw} = {val}")

        elif inst == "OUT": 
            val = 1 if cond_passed else 0
            self.mem.write(args.strip(), val)
            self.mem.append_display(f"[OUT] {args} = {val}")
        elif inst == "TMR":
            self._handle_timer(args, cond_passed)


    def _handle_timer(self, args, cond_passed):
        # args: "T1 K2"   This is an On-Delay timer  T1 is off while timing and goes 1 at expiration
        #                 and stays there until the rung cond goes false.
        parts = [p.strip() for p in args.split()]
        timer_id = parts[0]
        current_val = self.mem.read_timer_acc(timer_id)

        # Use the resolver for the setpoint (handles K200)
        setpoint = self._resolve_constant(parts[1]) if len(parts) > 1 else 0

        if cond_passed:
            # Accumulate time (using 50ms as a standard scan increment for the sim)
            if current_val < setpoint:
                current_val += 50 
                if current_val > setpoint: current_val = setpoint
                self.mem.write_timer_acc(timer_id, current_val)
                
            # The 'T*' bit only becomes True when setpoint reached
            if current_val >= setpoint:
                self.mem.write(timer_id, 1)
                status = "DONE"
            else:
                self.mem.write(timer_id, 0)
                status = "TIMING"
        else:
            # Reset behavior: Rung False = Accumulator 0 and Done Bit 0
            self.mem.write_timer_acc(timer_id, 0)
            self.mem.write(timer_id, 0)
            status = "IDLE"

        self.mem.append_display(f"[TMR{timer_id}]: {current_val}/{setpoint}ms ({status})")
         

# --- 4. WEB DASHBOARD ---
app = Flask(__name__)
shared_mem = None
shared_config = {}

DASHBOARD_HTML = """
<!DOCTYPE html>
<html>
<head>
    <title>{{ config.name }} Dashboard</title>
    <script>
        var isStepMode = {{ 'true' if mem.step_mode else 'false' }};
        if (!isStepMode) {
            setInterval(function(){ location.reload(); }, 2000);
        }
        if (window.location.pathname !== '/') { window.history.replaceState({}, '', '/'); }
    </script>
    <style>
        body { font-family: 'Courier New', monospace; background: #0d0d0d; color: #00ff41; padding: 20px; line-height: 1.4; }
        h1 { color: #ffca00; border-bottom: 3px double #00ff41; padding-bottom: 10px; margin-bottom: 20px; }
        
        .panel { background: #1a1a1a; border: 1px solid #333; padding: 15px; margin-bottom: 25px; }
        .panel h3 { margin-top: 0; color: #ffca00; text-transform: uppercase; border-bottom: 1px solid #444; padding-bottom: 5px; margin-bottom: 15px; font-size: 1em; }
        
        .btn { background: #00ff41; color: #000; border: none; padding: 8px 15px; cursor: pointer; font-family: inherit; font-weight: bold; }
        .btn-small { background: #00ff41; color: #000; border: none; padding: 4px 10px; cursor: pointer; font-size: 0.8em; font-weight: bold; }
        
        .light-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
        
        .bulb { width: 20px; height: 20px; border-radius: 50%; background: #222; border: 2px solid #333; }
        .bulb.on { background: #ffca00; box-shadow: 0 0 15px #ffca00; border-color: #fff; }
        
        .indicator { width: 10px; height: 10px; border-radius: 50%; background: #333; }
        .indicator.on { background: #00ff41; box-shadow: 0 0 8px #00ff41; }
        .indicator.prev-on { background: #00bfff; box-shadow: 0 0 8px #00bfff; }
        
        input { background: #000; border: 1px solid #00ff41; color: #00ff41; padding: 5px; font-family: inherit; }
        .label { font-size: 0.75em; color: #888; }
        
        /* Logic Trace Styling */
        .trace-container { font-family: monospace; margin-top: 15px; padding: 15px; background: #000; border: 1px solid #444; }
        .trace-line { display: block; margin-bottom: 4px; border-left: 2px solid #333; padding-left: 10px; }
        .trace-success { color: #00ff41; }
        .trace-fail { color: #ff4500; }
        
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 8px; border: 1px solid #333; text-align: center; }
        
        .toggle-btn { padding: 6px 12px; background: #222; border: 1px solid #444; color: #888; cursor: pointer; }
        .toggle-btn.on { border-color: #00ff41; color: #00ff41; }

        .error-box {
            background: #330000;
            border: 2px solid #ff4500;
            color: #ffca00;
            padding: 15px;
            margin-bottom: 20px;
            font-family: monospace;
            transition: background 0.2s;
        }
        .error-box:hover {
            background: #4a0000;
        }
        .dismiss-btn {
            position: absolute;
            top: 5px;
            right: 5px;
            background: #ff4500;
            color: #fff;
            border: none;
            padding: 2px 8px;
            cursor: pointer;
            font-weight: bold;
        }
    </style>
</head>
<body>
    <h1>{{ config.name }} INTERACTIVE VIEW</h1>

    {% if mem.syntax_errors %}
    <div class="error-box">
        <form action="/clear_errors" method="post" style="margin: 0;">
            <button type="submit" class="dismiss-btn" title="Dismiss">X</button>
        </form>
        <h3 style="margin: 0 0 10px 0; color: #ff4500;">⚠️ LOGIC PARSE ERRORS</h3>
        <div style="user-select: text;"> {% for error in mem.syntax_errors %}
                <div style="margin-bottom: 5px;">• {{ error }}</div>
            {% endfor %}
        </div>
    </div>
    {% endif %}

    <div class="panel" style="border-left: 5px solid #00ff41;">
        <div style="display: flex; gap: 20px; align-items: center; margin-bottom: 15px;">
             <form action="/toggle_speed" method="post">
                <button type="submit" class="btn">
                    {{ 'FAST MODE (50ms)' if mem.slow_mo_active else 'SLOW-MO MODE (1s)' }}
                </button>
            </form>
            <span>Mode: <b>{{ 'DEBUG SLOW-MO' if mem.slow_mo_active else 'REAL-TIME' }}</b></span>
        </div>

        <div style="display: flex; flex-wrap: wrap; gap: 15px; align-items: center; background: #000; padding: 10px; border: 1px solid #444;">
            <form action="/toggle_step" method="post"><button type="submit" class="btn">{{ 'EXIT STEP MODE' if mem.step_mode else 'ENTER STEP MODE' }}</button></form>
            
            {% if mem.step_mode %}
                <form action="/step" method="post"><button type="submit" class="btn" style="background: #ff00ff;">STEP</button></form>
                <form action="/next_scan" method="post"><button type="submit" class="btn" style="background: #00bfff;">SCAN</button></form>
                
                <form action="/set_checkpoint" method="post" style="display: flex; align-items: center; gap: 5px; border-left: 1px solid #333; padding-left: 10px;">
                    <span class="label">RUN TO:</span>
                    <input type="text" name="expr" value="{{ mem.breakpoint_expr }}" style="width: 100px;">
                    <button type="submit" class="btn-small" style="background: {{ '#ff00ff' if mem.breakpoint_in_progress else '#00ff41' }};">
                        {{ 'RUNNING...' if mem.breakpoint_in_progress else 'GO' }}
                    </button>
                </form>

                <div style="border-left: 1px solid #333; padding-left: 10px; display: flex; align-items: center; gap: 5px;">
                    <span class="label">EDIT:</span>
                    <form action="/edit_reg" method="post" style="display: flex; gap: 5px;">
                        <input type="text" name="reg" placeholder="Reg" style="width:50px;">
                        <input type="text" name="val" placeholder="Val" style="width:50px;">
                        <button type="submit" class="btn-small">SET</button>
                    </form>
                </div>

                <form action="/reset_cycle" method="post">
                    <button type="submit" class="btn" style="background: #ff4500;">REBOOT</button>
                </form>
                <form action="/manual_save" method="post">
                    <button type="submit" class="btn" style="background: #ffca00; color: #000;">SAVE RETENTION</button>
                </form>
            {% endif %}
        </div>

        <div class="trace-container">
            <div style="color: #ffca00; font-weight: bold; margin-bottom: 5px;">
                LAST: {{ mem.last_rung | safe }}
            </div>
            
            <div style="margin-left: 25px; border-left: 1px solid #333; padding-left: 10px;">
                {% if mem.last_details %}
                    {% for detail in mem.last_details %}
                        <div class="trace-line {{ 'trace-success' if detail[1] else 'trace-fail' }}" style="margin-top: 2px;">
                            <br><span style="color: #444;">↳</span> 
                            <b>{{ '✓' if detail[1] else '✗' }}</b> {{ detail[0] }}
                        </div>
                    {% endfor %}
                {% endif %}
            </div>

            <div style="color: #00bfff; margin-top: 15px; border-top: 1px solid #222; padding-top: 10px; font-size: 0.85em;">
                NEXT: {{ mem.next_rung }}
            </div>
        </div>

    <div class="panel">
        <h3>System Registers & Control Bits</h3>
        <div style="display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 15px;">
            {% for b in ['T1', 'T2', 'C151', 'C152', 'C153', 'C154', 'C1002', 'C1003', 'C1005', 'C1008', 'C1009', 'C1010', 'C1000'] %}
            <form action="/toggle_bit/{{b}}" method="post">
                <button type="submit" class="toggle-btn {{ 'on' if mem.read(b) else 'off' }}">{{ b }}</button>
            </form>
            {% endfor %}
        </div>
        <div style="display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 15px;">
            {% for ds in ['DS99','DS2', 'DS3', 'DS4', 'DS5', 'DS21', 'DH49', 'DS56'] %}
                <div> {{ds}} = {{ mem.read(ds) }}</div>
            {% endfor %}
        </div>
    </div>
    <div class="panel">
        <h3>Light Status  (C101-C124), Map (DS1000) ONRequest (C201-C224) OFFRequest (C251-264)</h3>
        <div class="light-grid">
            {% for i in range(1, 25) %}
            {% set c_addr = 101 + (i-1) %}
            {% set ds_map = 1000 + (i-1) %}   {# Mapping: DS1000-1023 #}
            {% set c_reqON = 200 + (i) %}     {# ReqON: C201-224 #}
            {% set c_reqOFF = 250 + (i) %}    {# ReqOFF: C251-274 #}
            {% set ds_QROff = 940 + (i) %}    {# QROff: DS941-964 #}

            {# Calculate Y addresses. which 16-bit bank we are in #}
            {# Lights 1-8 -> Y1xx, 9-16 -> Y2xx, 17-24 -> Y3xx #}
            {% set bank = ((i-1) // 8) + 1 %}
            {% set pair_idx = ((i-1) % 8) %}
    
            {% set y_on = (bank * 100) + (2 * pair_idx + 1) %}
            {% set y_off = (bank * 100) + (2 * pair_idx + 2) %}

            <div style="border: 1px solid #333; padding: 10px; background: #0a0a0a;">
                <div style="display: flex; justify-content: space-between; font-size: 0.7em; color: #666; margin-bottom: 5px;">
                    <span style="color: #ffca00;">PLC{{ config.slave_id }}-Y{{ y_on }}</span>
                    <span>C{{ c_addr }}</span>
                </div>
                <div style="display: flex; justify-content: center; margin-bottom: 10px;">
                    <div class="bulb {{ 'on' if mem.read('C' + (c_addr|string)) else 'off' }}"></div>
                </div>
                <div style="font-size: 0.7em; border-top: 1px solid #222; padding-top: 5px;">
                    <div style="color: #00ff41;">MAP: <nbsp> {{ mem.read('DS' + (ds_map|string)) }}</div>
                    <div style="color: #00bfff;">ReqON: <nbsp>{{ mem.read('C' + (c_reqON|string)) }}</div>
                    <div style="color: #00bfff;">ReqOFF: {{ mem.read('C' + (c_reqOFF|string)) }}</div>
                    <div style="color: #00bfff;">QROff: {{ mem.read('DS' + (ds_QROff|string)) }}</div>
                </div>
            </div>
            {% endfor %}
        </div>
    </div>

    <div class="panel">
        <h3>Schedule States (DS1051 - DS1062)</h3>
        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(100px, 1fr)); gap: 10px;">
            {% for i in range(1, 13) %}
            {% set ds_addr = 1050 + i %}
            {% set state = mem.read('DS' + (ds_addr|string)) %}
            <div style="border: 1px solid #333; padding: 10px; text-align: center; background: #0a0a0a; border-top: 3px solid {{ '#444' if state == 0 else '#00ff41' if state == 1 else '#00bfff' }};">
                <div class="label" style="margin-bottom: 5px;">SCH {{ i }} (DS{{ ds_addr }})</div>
                <div style="font-size: 1.1em; font-weight: bold; 
                    color: {{ '#444' if state == 0 else '#00ff41' if state == 1 else '#00bfff' }};">
                    {% if state == 0 %}
                        OFF
                    {% elif state == 1 %}
                        AUTO
                    {% elif state == 2 %}
                        QR-EN
                    {% else %}
                        UNKNOWN ({{ state }})
                    {% endif %}
                </div>
            </div>
            {% endfor %}
        </div>
    </div>


    <div class="panel">
        <h3>Active Schedule Data</h3>
        <div style="max-height: 400px; overflow-y: auto;">
            <table>
                <thead style="position: sticky; top: 0; background: #000;">
                    <tr>
                        <th>Sched/Span</th>
                        <th>Days (DS+0)</th>
                        <th>OnTrig (DS+1)</th>
                        <th>OnTime (DS+2)</th>
                        <th>OffTrig (DS+3)</th>
                        <th>OffTime (DS+4)</th>
                    </tr>
                </thead>
                <tbody>
                    {% set ns = namespace(found=false) %}
                    {% for s in range(1, 13) %}
                        {% for p in range(0, 14) %}
                            {% set base = (s-1)*70 + p*5 + 100 %}
                            {% if mem.read('DS'~base) or mem.read('DS'~(base+2)) or mem.read('DS'~(base+4)) %}
                                {% set ns.found = true %}
                                <tr>
                                    <td style="color: #ffca00;">S{{s}} P{{p}}</td>
                                    <td>{{ "0x%02X" | format(mem.read('DS'~base)) }}</td>
                                    <td>{{ mem.read('DS'~(base+1)) }}</td>
                                    <td style="color: #00bfff;">{{ mem.read('DS'~(base+2)) }}</td>
                                    <td>{{ mem.read('DS'~(base+3)) }}</td>
                                    <td style="color: #ff4500;">{{ mem.read('DS'~(base+4)) }}</td>
                                </tr>
                            {% endif %}
                        {% endfor %}
                    {% endfor %}
                </tbody>
            </table>
        </div>
    </div>
</body>
</html>
"""


@app.route('/')
def index():
    return render_template_string(DASHBOARD_HTML, mem=shared_mem, config=shared_config, hex=hex)

@app.route('/toggle_speed', methods=['POST'])
def toggle_speed():
    shared_mem.slow_mo_active = not shared_mem.slow_mo_active
    return redirect('/')

@app.route('/toggle_step', methods=['POST'])
def toggle_step():
    # 1.  flip the mode!
    shared_mem.step_mode = not shared_mem.step_mode
    shared_mem.breakpoint_in_progress = False

    # 2. If we are LEAVING step mode, nudge the engine to break the 'while' loop
    if not shared_mem.step_mode:
        shared_mem.step_trigger = True
        shared_mem.is_paused = False
        logger.info("UI: Exiting Step Mode -> Free Run Started")
    else:
        # Entering Step Mode
        shared_mem.step_trigger = False
        logger.info("UI: Entering Step Mode -> Paused")

    return redirect('/')

@app.route('/step', methods=['POST'])
def step():
    logger.info("UI: Step" )
    shared_mem.step_trigger = True
    return wait_and_render(timeout=2.0)

@app.route('/next_scan', methods=['POST'])
def next_scan():
    logger.info("UI: Next Scan" )
    shared_mem.scan_trigger = True
    return wait_and_render(timeout=2.0)

@app.route('/reset_cycle', methods=['POST'])
def reset_cycle():
    logger.info("UI: Reboot" )
    # 1. Clear volatile memory based on config ranges
    shared_mem.clear_volatile_memory(shared_config)
    shared_mem.clear_errors()
    
    # 2. Re-apply essential startup values
    shared_mem.first_scan_done = False 
    shared_mem.step_mode = True
    
    # 3. Signal the logic engine to teleport to the start
    shared_mem.logic_reset_requested = True

    # 4. BARRIER: Wait for the engine to hit the pause at Rung 1
    return wait_and_render(timeout=3.0)

@app.route('/edit_reg', methods=['POST'])
def edit_reg():
    reg = request.form.get('reg').upper()
    raw_val = request.form.get('val')
    val = 0;
    if not reg:
        logger.warning("UI: Edit attempted with no register label.")
        return redirect('/')
    try:
        # 2. Handle empty or non-numeric values gracefully
        # If the string is empty, we'll treat it as 0. 
        # If it's hex (like 0x10) or int, this logic handles it.
        if not raw_val:
            val = 0
        elif raw_val.lower().startswith('0x'):
            val = int(raw_val, 16)
        else:
            # We use float then int to handle cases where users might type '1.0'
            val = int(float(raw_val))

        # 3. Perform the write
        shared_mem.write(reg, val)
        logger.info(f"UI: Manually set {reg} = {val}")

    except ValueError:
        logger.error(f"UI: Invalid value entered for {reg}: '{raw_val}'")
        # Optional: You could pass an error message back to the UI here
    return redirect('/')

@app.route('/set_checkpoint', methods=['POST'])
def set_checkpoint():
    expr = request.form.get('expr').strip()
    if expr:
        # 1. Store the expression
        shared_mem.breakpoint_expr = expr
        
        # 2. Syntax Check: Try to parse it once before committing
        # We use a dummy check just to see if the regex matches
        match = re.search(r"([A-Z0-9\[\]]+)\s*([><=!]+)\s*([A-Z0-9]+)", expr)
        if not match:
            # You could add a flash message here if using Flask sessions, 
            # for now, we'll just log it and not start the run.
            logger.error(f"UI: Rejected invalid breakpoint syntax: {expr}")
            return redirect('/')

        # 3. Start the run
        shared_mem.breakpoint_in_progress = True
        shared_mem.step_mode = False  # Set to run full speed
        shared_mem.step_trigger = True # Release current pause
    return redirect('/')

@app.route('/clear_errors', methods=['POST'])
def clear_errors():
    shared_mem.clear_errors()
    return redirect('/')

def wait_and_render(timeout=2.0):
    """Blocks the Flask thread until the Logic Engine reports is_paused=True,
       then returns the rendered dashboard HTML."""

    # give the engine an oppertunity to start moving in the other thread.
    time.sleep(0.05)
    
    # Barrier: Wait for the engine to reach a stable pause point
    start_wait = time.time()
    while not shared_mem.is_paused and (time.time() - start_wait < timeout):
        time.sleep(0.01)
        
    # Once we are here, is_paused is True, meaning the engine is sleeping
    # and the Memory Map is stable for sampling.
    return render_template_string(DASHBOARD_HTML, mem=shared_mem, config=shared_config, hex=hex)

@app.route('/manual_save', methods=['POST'])
def manual_save():
    shared_mem.save_to_disk(shared_config)
    return redirect('/')



def start_dashboard(mem, config):
    global shared_mem, shared_config
    shared_mem = mem
    shared_config = config
    # Web port = Modbus port + 3000 (e.g., 8020)
    web_port = config['port'] + 3000
    app.run(host='0.0.0.0', port=web_port, debug=False, use_reloader=False, threaded=True)

def start_modbus_thread(context,  config):
    """Lane 1: Modbus Server (Background Thread)"""
    identity = ModbusDeviceIdentification()
    identity.VendorName = 'CLICK_Sim'
    identity.ProductCode = config['name']
    identity.ModelName = 'CLICK PLUS'

    # Asyncio needs its own loop inside a new thread
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    
    logger.info(f"Starting Modbus for {config['name']} on port {config['port']} with Slave ID {config['slave_id']}")
    loop.run_until_complete(StartAsyncTcpServer(
        context=context,
        identity=identity,
        address=("", config['port'])
    ))


def run_loop(mem, engine, config):
    logger.info("Logic Engine Started.")
    while True:
        try:
            # If in step mode, slow down the check to save CPU
            sleep_time = 0.5 if mem.step_mode else 0.05
                
            # This executes MAIN and any CALLs
            engine.run_scan(slow_mo=mem.slow_mo_active)
                
            time.sleep(sleep_time)

            if mem.needs_save:
                mem.save_to_disk(config)
                mem.needs_save = False # Reset the flag

                
        except LogicResetException:
            # This is where we land after 'raise LogicResetException()'
            logger.info("!!! Logic RESET Caught: Rewinding to MAIN !!!")
            mem.logic_reset_requested = False
            continue 
        except Exception as e:
            fault = getattr(mem, 'next_rung', 'Unknown')
            logger.error(f"Logic Loop Error at {fault}: {e}")
            time.sleep(1)



def run_plc():
    if len(sys.argv) < 2:
        print("Usage: python3 soft_plc.py <config.json>")
        return

    with open(sys.argv[1]) as f:
        config = json.load(f)

    # Modbus Setup
    store = ModbusSlaveContext(di=ModbusSequentialDataBlock(0, [0]*65535),
                               co=ModbusSequentialDataBlock(0, [0]*65535),
                               hr=ModbusSequentialDataBlock(0, [0]*65535),
                               ir=ModbusSequentialDataBlock(0, [0]*65535))
    context = ModbusServerContext(slaves={config['slave_id']: store, 0x00: store}, single=False)
    

    # Apply initial state from config
    mem = MemoryManager(context, config['slave_id'], config=config)
    for k, v in config.get('initial_state', {}).items(): mem.write(k.replace('_stub',''), 1 if v else 0)

    # 1. Initialize Engine
    programs = CLICKParser(mem).parse(config['ladder_file'])
    engine = LogicEngine(mem, programs)

    # 2. Launch the modbus server with the specific ID from your config  (THREAD A)
    threading.Thread(target=start_modbus_thread, 
                     args=(context, config), 
                     daemon=True).start()

    # 3. Start Dashboard Thread  (THREAD B)
    threading.Thread(target=start_dashboard, 
                     args=(mem, config), 
                     daemon=True).start()


    # 4. start the Logic Loop in foreground   (MAIN THREAD)
    run_loop(mem, engine, config)

if __name__ == "__main__":
    run_plc()

