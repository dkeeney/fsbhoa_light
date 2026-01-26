import re
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
        # CLICK to Modbus Offset Map (0-based)
        self.map = {'X': 0, 'Y': 8192, 'C': 16384, 'DS': 0, 'TD': 45056, 'SD': 0, 'DH': 0, 'YD': 8192}
        # Initialize memory immediately on startup
        if config:
            self.clear_volatile_memory(config)

    def clear_volatile_memory(self, config):
        """Standardizes all registers to 0 except for defined retentive ranges."""
        retentive = config.get('retentive', {})
        
        # Expanded prefixes to cover all CLICK register types we use
        prefixes = ['C', 'DS', 'DH', 'Y', 'T', 'CT']
        
        for prefix in prefixes:
            # Resolve the first address to get m_type and base_addr
            m_type, base_addr = self._resolve_label(f"{prefix}1")
            
            for i in range(1000):
                addr = base_addr + i
                reg_num = i + 1  # 1-based index (e.g., DS1, DS2...)
                
                # Logic check: Should this specific register stay?
                is_sticky = False
                if prefix in retentive:
                    start, end = retentive[prefix]
                    if start <= reg_num <= end:
                        is_sticky = True

                if not is_sticky:
                    # Explicitly write a 0 to ensure it's an integer, not None

                    self.context[self.slave_id].setValues(m_type, addr, [0])
        
        # Reset System Flags
        self.first_scan_done = False
        logger.info("Cold Reboot: Non-retentive memory cleared to 0.")

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
            self.last_rung += f"<br>&nbsp;&nbsp;{self.rung_id} <span style='color:#00ffff'>» {text}</span>"
            logger.info(f"{self.rung_id} {text}")

# --- 2. CLICK PARSER ---
class CLICKParser:
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
                    "conds": self._parse_recursive_conds(c_part),
                    "acts": self._parse_acts(a_part)
                })
                
                # Store the original text for the Dashboard diagnostic
                programs["__raw__"][block].append(text)

    def _parse_recursive_conds(self, text):
        results = []
        # Find top-level bracketed groups: looks for [ ... ] 
        # but handles cases where brackets are adjacent
        tokens = re.findall(r"\[([^\[\]]+|\[[^\[\]]+\])\]", text)
        
        for t in tokens:
            t = t.strip()
            if t.startswith("OR "):
                # Recursive call for content after "OR "
                sub = self._parse_recursive_conds(t[3:])
                results.append({"type": "OR", "val": sub})
            elif t.startswith("AND "):
                sub = self._parse_recursive_conds(t[4:])
                results.append({"type": "AND", "val": sub})
            elif t.startswith("NOT "):
                results.append({"type": "NOT", "val": t[4:].strip()})
            elif t.startswith("CMP "):
                results.append({"type": "CMP", "val": t[4:].strip()})
            else:
                # Handle simple group logic: [C1 C2] -> AND group
                if " " in t:
                    sub_parts = [{"type": "DIRECT", "val": x} for x in t.split()]
                    results.append({"type": "AND", "val": sub_parts})
                else:
                    results.append({"type": "DIRECT", "val": t})
        return results

    def _parse_acts(self, text):
        acts = []
        
        # 1. Strip comments first as requested
        # This removes everything from // to the end of the line
        clean_text = re.sub(r"//.*$", "", text).strip()
        
        # 2. Extract balanced parentheses blocks
        # This regex looks for: (Instruction ARGS)
        # It handles nested parentheses by looking for the outermost pairs
        pattern = r"\((\w+)(?:\s+((?:[^\(\)]|\([^\(\)]*\))*))?\)"
        raw_acts = re.findall(pattern, clean_text)
        
        for inst, args in raw_acts:
            inst = inst.upper()
            args = args.strip()
            
            # Handle the specific case of (parallel) -> which is a marker, not an act
            if inst == "PARALLEL":
                continue
                
            acts.append({"inst": inst, "args": args})
            
        return acts

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
                    display_text += f"\n   <span style='color:{color}'>↳ {text}</span>"
            
                self.mem.last_rung = display_text


            # --- LANDMARK 5: Execute Actions if Conditions Passed ---
            for act in rung['acts']:
                inst = act['inst']
                args = act['args']

                if inst == "FOR":
                    count_match = re.search(r'\d+', args)
                    count = int(count_match.group()) if count_match else 1
                    loop_stack.append({"start": ptr, "curr": 1, "max": count})
                    self.mem.append_display(f"[FOR {count}]")
                    
                elif inst == "NEXT":
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


    def eval_conds(self, conds):
        # Returns (Overall_Boolean, List_of_Diagnostic_Strings)
        details = []
        overall_result = True

        for c in conds:
            c_type = c['type']
            label = c['val']
            success = False

            if c_type == "DIRECT":
                success = self.mem.read(label)
                details.append((f"{label}", success))
            
            elif c_type == "NOT":
                success = not self.mem.read(label)
                details.append((f"NOT {label}", success))
            
            elif c_type == "CMP":
                # eval_cmp now returns (bool, "2 > 24")
                success, math_str = self.eval_cmp(label)
                details.append((f"CMP {math_str}", success))
            
            elif c_type == "OR":
                # Recursive call for the OR group
                success, sub_details = self.eval_conds(label)
                # Format OR group for dashboard
                details.append(("OR GROUP", success))
                # Add nested details indented
                for sub_label, sub_success in sub_details:
                    details.append((f"  + {sub_label}", sub_success))
            
            elif c_type == "AND":
                success, sub_details = self.eval_conds(label)
                if len(sub_details) > 1:
                    details.append(("AND GROUP", success))
                for sub_label, sub_success in sub_details:
                    details.append((f"  & {sub_label}", sub_success))

            if not success:
                overall_result = False
                # We stop processing the AND chain at the first failure
                break

        return overall_result, details

    def eval_cmp(self, expr):
        try:
            # 1. Flexible Regex for registers, constants, and indices
            match = re.search(r"([A-Z0-9\[\]]+)\s*([><=!]+)\s*([A-Z0-9]+)", expr)
            if not match:
                logger.warning(f"CMP Regex Failed: '{expr}'")
                return False, f"Regex Fail: {expr}"

            v1_label, op, v2_label = match.groups()

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
            trace = f"{v1_label}({n1}) {op} {n2}"
        
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
                return int(val_str[1:])
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
                    match = re.search(r'([A-Z]+)\[([A-Z]+\d+)\]', label)
                    if match:
                        base_type, index_reg = match.groups()
                        index_val = self.mem.read(index_reg)
                        return f"{base_type}{index_val}"
                    return label

                # 1. Resolve source and destination addresses
                actual_src_addr = resolve_indexed(src_str)
                actual_dest_addr = resolve_indexed(dest_str)

                # 2. Get the value from source
                val = self._resolve_constant(actual_src_addr)

                # 3. Write to destination
                self.mem.write(actual_dest_addr, val)
                
                if self.mem.step_mode:
                    self.mem.append_display(f"[COPY] {val} -> {actual_dest_addr} (Indexed: {dest_str})")
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
                calc_expr = re.sub(r'\bMOD\b', '%', calc_expr, flags=re.IGNORECASE)

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
                    self.mem.append_display(f"[MATH] {target} = {result} (Source: {expr.strip()})")
                        
                except Exception as e:
                    logger.error(f"MATH Error: {target} = {expr} (Evaluated: {calc_expr}) -> {e}")
            else:
                logger.error(f"MATH Error: {target} = {expr} Missing =")
        elif cond_passed and inst == "SET": 
            self.mem.write(args.strip(), 1)
            self.mem.append_display(f"[SET] {args} = 1")
        elif cond_passed and inst == "RST": 
            self.mem.write(args.strip(), 0)
            self.mem.append_display(f"[RST] {args} = 0")
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
        // Only refresh every 2 seconds if NOT in Step Mode
        var isStepMode = {{ 'true' if mem.step_mode else 'false' }};
        console.log("PLC Mode initialized as: " + (isStepMode ? "STEP" : "RUN"));

        if (!isStepMode) {
            console.log("Auto-refresh started (2s interval)");
            setInterval(function(){
                location.reload();
            }, 2000);
        }
        // Clean up the URL so 'Next' doesn't stay in the address bar
        if (window.location.pathname !== '/') {
            window.history.replaceState({}, '', '/');
        }
    </script>
    <style>
        body { font-family: 'Courier New', monospace; background: #0d0d0d; color: #00ff41; padding: 20px; }
        .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 10px; margin-bottom: 20px;}
        .box { border: 1px solid #00ff41; padding: 15px; background: #1a1a1a; text-align: center; }
        .on { background: #00ff41; color: #000; font-weight: bold; }
        .btn { background: #00ff41; color: #000; border: none; padding: 10px 20px; cursor: pointer; font-family: inherit; font-weight: bold; }
        .label { font-size: 0.8em; display: block; color: #888; margin-bottom: 5px; }
        h2 { border-bottom: 2px solid #00ff41; padding-bottom: 5px; margin-top: 30px; }

        .btn-small { background: #00ff41; color: #000; border: none; padding: 2px 8px; cursor: pointer; font-size: 0.8em; font-weight: bold; }
        input { background: #1a1a1a; border: 1px solid #00ff41; color: #00ff41; padding: 2px; font-family: inherit; }
    </style>
</head>
<body>
    <h1>{{ config.name }} INTERACTIVE VIEW</h1>
    <div style="margin-bottom: 20px;">
        <form action="/toggle_speed" method="post" style="display:inline;">
            <button type="submit" class="btn">
                TOGGLE: {{ 'SWITCH TO 50ms' if mem.slow_mo_active else 'SWITCH TO SLOW-MO (1s)' }}
            </button>
        </form>
        <span style="margin-left: 20px;">Current Mode: <b>{{ 'SLOW-MOTION (Trace On)' if mem.slow_mo_active else 'REAL-TIME' }}</b></span>
    </div>
    <div style="background: #222; padding: 15px; border: 1px dashed #00ff41; margin-bottom: 20px;">
        <div style="margin-bottom: 10px;">
            <form action="/toggle_step" method="post" style="display:inline;">
                <button type="submit" class="btn">{{ 'EXIT STEP MODE' if mem.step_mode else 'ENTER STEP MODE' }}</button>
            </form>
            {% if mem.step_mode %}
            <form action="/step" method="post" style="display:inline; margin-left: 10px;">
                <button type="submit" class="btn" style="background: #ff00ff;">NEXT STEP</button>
            </form>
            <form action="/next_scan" method="post" style="display:inline; margin-left: 10px;">
                <button type="submit" class="btn" style="background: #00bfff;">NEXT SCAN</button>
            </form>
            <form action="/reset_cycle" method="post" style="display:inline; margin-left: 10px;">
                <button type="submit" class="btn" style="background: #ff4500;">RESET CYCLE</button>
            </form>
            {% endif %}
        </div>

        <div style="font-family: monospace; font-size: 0.9em; white-space: pre-wrap;">
            <div style="color: #ffca00; margin-bottom: 5px;">LAST: {{ mem.last_rung | safe }}</div>
            <div style="color: #00bfff;">NEXT: {{ mem.next_rung }}</div>
        </div>
        
        {% if mem.step_mode %}
        <div style="margin-top: 15px; border-top: 1px solid #444; pt-10px;">
            <span class="label">EDIT REGISTER (e.g., DS3):</span>
            <form action="/edit_reg" method="post" style="display:inline;">
                <input type="text" name="reg" placeholder="Label" style="width:60px;">
                <input type="number" name="val" placeholder="Value" style="width:60px;">
                <button type="submit" class="btn-small">SET</button>
            </form>
        </div>
        {% endif %}
    </div>
    
    <h2>Control Bits (C)</h2>
    <div class="grid">
        {% for i in range(1, 13) %}
        <div class="box {{ 'on' if mem.read('C'~i) }}">
            <span class="label">Sched {{i}}</span> C{{i}}
        </div>
        {% endfor %}
        <div class="box {{ 'on' if mem.read('C154') }}">
            <span class="label">Photocell</span> C154
        </div>
    </div>
    <h2>Light Output Status (C101-C124)</h2>
    <div class="grid">
        {% for i in range(101, 125) %}
        <div class="box {{ 'on' if mem.read('C'~i) }}">
            <span class="label">Light {{i-100}}</span> C{{i}}
        </div>
        {% endfor %}
    </div>

    <h2>Sequencer & Time</h2>
    <div class="grid">
        <div class="box"><span class="label">Sequencer</span> DS3: {{ mem.read('DS3') }}</div>
        <div class="box"><span class="label">PLC HHMM</span> DS30: {{ mem.read('DS30') }}</div>
        <div class="box"><span class="label">Day (Sun=1)</span> SD23: {{ mem.read('SD23') }}</div>
    </div>
    <h2>Internal Pointers & States</h2>
    <div class="grid">
        <div class="box"><span class="label">Sched Index (DS4)</span>{{ mem.read('DS4') }}</div>
        <div class="box"><span class="label">Span Index (DS2)</span>{{ mem.read('DS2') }}</div>
        <div class="box"><span class="label">Base Ptr (DS21)</span>{{ mem.read('DS21') }}</div>
        <div class="box"><span class="label">Day Mask (DH49)</span>{{ hex(mem.read('DH49')) }}</div>
    </div>
    <h2>Schedule Data (DS100-DS939)</h2>
    <div style="overflow-y: auto; max-height: 300px; background: #1a1a1a; border: 1px solid #444; padding: 10px;">
        <table style="width: 100%; border-collapse: collapse; font-size: 0.8em;">
            <thead style="position: sticky; top: 0; background: #000; color: #888;">
                <tr style="border-bottom: 1px solid #00ff41;">
                    <th style="text-align: left;">Sched/Span</th>
                    <th>Days (DS+0)</th>
                    <th>OnTrig (DS+1)</th>
                    <th>OnTime (DS+2)</th>
                    <th>OffTrig (DS+3)</th>
                    <th>OffTime (DS+4)</th>
                </tr>
            </thead>
            <tbody>
                {% for s in range(1, 13) %}
                    {% for p in range(0, 14) %}
                        {% set base = (s-1)*70 + p*5 + 100 %}
                        {# We show the row if any of the span data is non-zero #}
                        {% if mem.read('DS'~base) or mem.read('DS'~(base+2)) or mem.read('DS'~(base+4)) %}
                        <tr style="border-bottom: 1px solid #333;">
                            <td style="color: #ffca00;">S{{s}} P{{p}}</td>
                            <td style="text-align: center;">{{ hex(mem.read('DS'~base)) }}</td>
                            <td style="text-align: center;">{{ mem.read('DS'~(base+1)) }}</td>
                            <td style="text-align: center; color: #00bfff;">{{ mem.read('DS'~(base+2)) }}</td>
                            <td style="text-align: center;">{{ mem.read('DS'~(base+3)) }}</td>
                            <td style="text-align: center; color: #ff4500;">{{ mem.read('DS'~(base+4)) }}</td>
                        </tr>
                        {% endif %}
                    {% endfor %}
                {% endfor %}
            </tbody>
        </table>
        {% if not any_data_found %}
            <div style="color: #ff4500; text-align: center; padding: 20px;">
                *** NO SCHEDULE DATA DETECTED (DS100-DS939 ARE ALL ZERO) ***
            </div>
        {% endif %}
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
    shared_mem.step_trigger = True
    return wait_and_render(timeout=2.0)

@app.route('/next_scan', methods=['POST'])
def next_scan():
    shared_mem.scan_trigger = True
    return wait_and_render(timeout=2.0)

@app.route('/reset_cycle', methods=['POST'])
def reset_cycle():
    # 1. Clear volatile memory based on config ranges
    shared_mem.clear_volatile_memory(shared_config)
    
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
    val = int(request.form.get('val'))
    shared_mem.write(reg, val)
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


def run_loop(mem, engine):
    logger.info("Logic Engine Started.")
    while True:
            try:
                # If in step mode, slow down the check to save CPU
                sleep_time = 0.5 if mem.step_mode else 0.05
                
                # This executes MAIN and any CALLs
                engine.run_scan(slow_mo=mem.slow_mo_active)
                
                time.sleep(sleep_time)
                
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
    mem = MemoryManager(context, config['slave_id'])
    for k, v in config.get('initial_state', {}).items(): mem.write(k.replace('_stub',''), 1 if v else 0)

    # 1. Initialize Engine
    programs = CLICKParser().parse(config['ladder_file'])
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
    run_loop(mem, engine)

if __name__ == "__main__":
    run_plc()

