package bbcdisasm

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

// Which types of data should the visit callback to walk be invoked for.
// This has only the most basic support right now.
type visitMask uint

const (
	vtCode visitMask = 1 << iota
	vtData

	vtNone = visitMask(0)
	vtAll  = ^visitMask(0)
)

type varDef struct {
	Sval string
	Ival uint
}

// Disassembler converts byte code to a textual representation
type Disassembler struct {
	Program  []byte // The 6502 program to be disassembled
	MaxBytes uint   // Maximum number of program bytes to disassemble
	Offset   uint   // Starting offset to begin disassembly

	// The load address of the program, required to correctly compute absolute
	// addresses for relative branches.
	BranchAdjust uint

	// The set of addresses in the program that the disassembler should ensure
	// to disassemble. This is useful in cases where the disassembler skips
	// addresses due to misinterpreting data bytes as opcodes.
	// Will be modified by Disassemble().
	CodeAddrs []uint

	usedOSAddress map[uint]bool
	usedOSVector  map[uint]bool
	branchTargets map[uint]int
	vars          map[string]varDef
}

// NewDisassembler initializes a new Disassembler with the target progrsm
func NewDisassembler(program []byte) *Disassembler {
	return &Disassembler{
		Program:       program,
		usedOSAddress: make(map[uint]bool),
		usedOSVector:  make(map[uint]bool),
		vars:          make(map[string]varDef),
	}
}

// AddVar defines a new variable. The disassembler will include the definition
// at the top of the disassembly and refer to matching value by name.
func (d *Disassembler) AddVar(name, value string) error {
	var base int
	ovalue := value

	// If the value starts with a & then assume it is an hexadecimal value
	// in the idiomatic form of BBC BASIC.
	if value[0] == '&' {
		value = strings.Trim(value, "&")
		base = 16
	}
	ival, err := strconv.ParseInt(value, base, 0)
	if err != nil {
		return err
	}
	d.vars[name] = varDef{ovalue, uint(ival)}
	return nil
}

func (d *Disassembler) walk(vm visitMask, fn func(cursor uint, codeAddrIdx int, b byte, op Opcode, opOk bool) int) {
	cursor := d.Offset
	prevCur := cursor
	codeAddrIdx := 0
	for cursor < (d.Offset + d.MaxBytes) {
		// Do we have remaining code addresses?
		if codeAddrIdx < len(d.CodeAddrs) {
			// If we overstepped the next code address, pull the cursor back
			ca := d.CodeAddrs[codeAddrIdx]
			if prevCur < ca && cursor >= ca {
				cursor = ca
				codeAddrIdx++
			}
		}
		prevCur = cursor

		// All instructions are at least one byte long and the first byte is
		// sufficient to identify the opcode.
		b := d.Program[cursor]
		op, opOk := OpCodesMap[b]

		// If the decoded 'instruction' straddles a code address then treat it
		// as data.
		if opOk && codeAddrIdx < len(d.CodeAddrs) {
			if cursor+op.Length >= d.CodeAddrs[codeAddrIdx] {
				if vm&vtData == 0 {
					cursor = d.CodeAddrs[codeAddrIdx]
					continue
				}
			}
		}

		cursor += uint(fn(cursor, codeAddrIdx, b, op, opOk))
	}
}

// Disassemble a 6502 program into a textual representation written to w
// offset is where disassembly starts from the beginning of program.
// branchAdjust is used to adjust the target address of relative branches to a
// 'meaningful' address, typically the load address of the program.
func (d *Disassembler) Disassemble(w io.Writer) {
	if len(d.CodeAddrs) > 0 {
		sort.Slice(d.CodeAddrs, func(i, j int) bool { return d.CodeAddrs[i] < d.CodeAddrs[j] })

		for i, ca := range d.CodeAddrs {
			d.CodeAddrs[i] = ca - d.BranchAdjust
		}
	}

	// First pass through program is to find the location of any branches. These
	// will be marked as labels in the output.
	d.findBranchTargets()

	distem, _ := template.New("disasm").Parse(disasmHeader)
	data := struct {
		UsedOSAddress map[uint]bool
		OSAddress     map[uint]string
		UsedOSVector  map[uint]bool
		OSVector      map[uint]string
		Vars          map[string]varDef
		LoadAddr      uint
	}{d.usedOSAddress, addressToOsCallName, d.usedOSVector, osVectorAddresses, d.vars, d.BranchAdjust}
	if err := distem.Execute(w, data); err != nil {
		panic(err)
	}

	// Second pass through program is to decode each instruction
	// and print to stdout.
	d.walk(vtAll, func(cursor uint, codeAddrIdx int, b byte, op Opcode, opOk bool) int {
		var sb strings.Builder
		if targetIdx, ok := d.branchTargets[cursor+d.BranchAdjust]; ok {
			sb.WriteByte('.')
			sb.WriteString(fmt.Sprintf(labelFormatString, targetIdx))
			sb.WriteString("\n")
			w.Write([]byte(sb.String()))

			sb.Reset()
		}
		sb.WriteByte(' ')

		var advance uint

		// Situations that can arise decoding the next instruction
		// 1) If the byte does not match an opcode - print as data
		// 2) If the byte matches a documented opcode:
		//    a) If the instruction won't assemble identically then print as
		//       data.
		//    b) If the instruction straddles a targeted code address then print
		//       as data the bytes up to the targeted address.
		//    c) Otherwise, decode operands and print.
		// 3) If the byte matches an undocumented opcode:
		//    a) If the instruction straddles a targeted code address then print
		//       as data the bytes up to the targeted address.
		//    b) Otherwise retrieve operands, print as data, mark UD
		if opOk {
			instruction := d.Program[cursor : cursor+op.Length]
			doc := isOpcodeDocumented(op)
			wai := willAssembleIdentically(op, instruction)

			var straddles bool
			if codeAddrIdx < len(d.CodeAddrs) {
				straddles = cursor+op.Length >= d.CodeAddrs[codeAddrIdx]
			}

			if doc && wai && !straddles {
				// If here then documented instruction that will assemble correctly
				d.printInstruction(&sb, op, instruction, cursor)

				advance = op.Length
			} else {
				// The opcode was unrecognized, the opcode belongs to an
				// undocumented instruction, the instruction will straddle a
				// targeted code address or beebasm will not assemble
				// to the same bytes. In these cases treat it as data.

				// If the data block straddles a targeted code address then trim
				// to the address.
				if straddles {
					nb := d.CodeAddrs[codeAddrIdx] - cursor
					instruction = instruction[:nb]
				}

				// Include data bytes in comment section for visual consistency
				// if the instruction is documented. Non documented instructions
				// will print something else.
				printData(&sb, instruction, doc, cursor+d.BranchAdjust)

				if !doc {
					// Undocumented instruction includes additional info before printable bytes
					// EQUB [opcode],...,[opcode] \ [address] UD [instruction mnemonic]   [printable bytes]
					//                            ^--- 25th column                        ^--- 45th column
					sb.WriteString("UD ")
					sb.WriteString(op.Name)
				}

				appendPrintableBytes(&sb, instruction)

				advance = uint(len(instruction))
			}
		} else {
			bs := []byte{b}
			printData(&sb, bs, true, cursor+d.BranchAdjust)
			appendPrintableBytes(&sb, bs)
			advance = 1
		}

		sb.WriteByte('\n')
		w.Write([]byte(sb.String()))

		return int(advance)
	})
}

func (d *Disassembler) printInstruction(sb *strings.Builder, op Opcode, instruction []byte, cursor uint) {
	// A valid instruction will be printed to a line with format
	//
	// [instruction mnemonic]     \ [address] [instruction opcodes]   [printable bytes]
	//                            ^--- 25th column                    ^--- 45th column
	sb.WriteString(op.Name)
	sb.WriteByte(' ')
	sb.WriteString(d.decode(op, instruction, cursor))

	appendSpaces(sb, max(24-sb.Len(), 1))
	sb.WriteString("\\ ")

	out := []string{
		fmt.Sprintf("&%04X", cursor+d.BranchAdjust),
	}
	for _, i := range instruction {
		out = append(out, fmt.Sprintf("%02X", i))
	}
	sb.WriteString(strings.Join(out, " "))

	appendPrintableBytes(sb, instruction)
}

// Print data in hex as comma-delimited EQUB statement. Assumes that there are
// between 1 and 3 data bytes though it will handle any amount.
// If bytesInComment is true then the data byte values will be repeated in the
// comment section.
func printData(sb *strings.Builder, data []byte, bytesInComment bool, address uint) {
	// Data will be printed to a line with format
	// EQUB &[byte],...,&[byte]    \ [address] [byte] ... [byte] [printable bytes]
	//                             ^--- 25th column              ^--- 45th column
	var out []string
	for _, i := range data {
		out = append(out, fmt.Sprintf("&%02X", i))
	}
	sb.WriteString("EQUB ")
	sb.WriteString(strings.Join(out, ","))

	appendSpaces(sb, max(24-sb.Len(), 1))
	sb.WriteString("\\ ")

	out = []string{fmt.Sprintf("&%04X", address)}
	if bytesInComment {
		for _, i := range data {
			out = append(out, fmt.Sprintf("%02X", i))
		}
	}
	sb.WriteString(strings.Join(out, " "))
	sb.WriteByte(' ')
}

func appendSpaces(sb *strings.Builder, ns int) {
	sb.Write(bytes.Repeat([]byte{' '}, ns))
}

func appendPrintableBytes(sb *strings.Builder, b []byte) {
	appendSpaces(sb, max(44-sb.Len(), 1))
	for _, c := range b {
		sb.WriteByte(toChar(c))
	}
}

func toChar(b byte) byte {
	if b < 32 || b > 126 {
		return '.'
	}
	return b
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func isOpcodeDocumented(op Opcode) bool {
	for _, u := range UndocumentedInstructions {
		if op.Name == u {
			return false
		}
	}

	return true
}

// willAssembleIdentically checks if beebasm will assemble the instruction as written
//
// Given an instruction with a 16-bit absolute address operand that lies in the
// Zero Page e.g. LDA &0012, beebasm will instead assemble using the zero page
// form if supported, e.g. LDA &12. This behavior breaks binary compatibility.
func willAssembleIdentically(op Opcode, instruction []byte) bool {
	if op.AddrMode == Absolute || op.AddrMode == AbsoluteX || op.AddrMode == AbsoluteY {
		tgt := (uint(instruction[2]) << 8) + uint(instruction[1])
		if tgt < 0x100 {
			return false
		}
	}

	return true
}

func (d *Disassembler) decode(op Opcode, bytes []byte, cursor uint) string {
	// Jump and Branch instructions have special handling
	if bytes[0] == OpJMPAbsolute || bytes[0] == OpJSRAbsolute {
		// JMP &1234 and JSR &1234 are special cased with naming for well known
		// OS call entry points.
		return genAbsoluteOsCall(bytes, d.branchTargets)
	}
	if op.branchOrJump() == btBranch {
		return genBranch(bytes, cursor, d.BranchAdjust, d.branchTargets, op.Length)
	}

	switch op.AddrMode {
	case None:
		return ""
	case Accumulator:
		return "A"
	case Immediate:
		return fmt.Sprintf("#&%02X", bytes[1])
	case Absolute:
		val := (uint(bytes[2]) << 8) + uint(bytes[1])

		// Look up in the OS vector address space
		if osv, ok := osVectorAddresses[val]; ok {
			return osv
		}
		// Try again with the bottom bit cleared because each vector is 16-bit
		// eg. USERV vector is at 0x200 and 0x201.
		if osv, ok := osVectorAddresses[val&^uint(1)]; ok {
			return osv + "+1"
		}

		if dvar, ok := d.lookupVar(val); ok {
			return dvar
		}

		// Unrecognized address, return as numeric
		return fmt.Sprintf("&%04X", val)
	case ZeroPage:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return dvar
		}
		return fmt.Sprintf("&%02X", bytes[1])
	case ZeroPageX:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return dvar + ",X"
		}
		return fmt.Sprintf("&%02X,X", bytes[1])
	case ZeroPageY:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return dvar + ",Y"
		}
		return fmt.Sprintf("&%02X,Y", bytes[1])
	case ZeroPageRel:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return fmt.Sprintf("%s, #%02x", dvar, bytes[2])
		}
		return fmt.Sprintf("&%02X,#%02X", bytes[1], bytes[2])
	case Indirect:
		val := (uint(bytes[2]) << 8) + uint(bytes[1])
		if dvar, ok := d.lookupVar(val); ok {
			return "(" + dvar + ")"
		}
		return fmt.Sprintf("(&%04X)", val)
	case AbsoluteX:
		val := (uint(bytes[2]) << 8) + uint(bytes[1])
		if dvar, ok := d.lookupVar(val); ok {
			return dvar + ",X"
		}
		return fmt.Sprintf("&%04X,X", val)
	case AbsoluteY:
		val := (uint(bytes[2]) << 8) + uint(bytes[1])
		if dvar, ok := d.lookupVar(val); ok {
			return dvar + ",Y"
		}
		return fmt.Sprintf("&%04X,Y", val)
	case IndirectX:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return "(" + dvar + ",X)"
		}
		return fmt.Sprintf("(&%02X,X)", bytes[1])
	case IndirectY:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return "(" + dvar + "),Y"
		}
		return fmt.Sprintf("(&%02X),Y", bytes[1])
	case IndirectZP:
		if dvar, ok := d.lookupVar(uint(bytes[1])); ok {
			return "(" + dvar + ")"
		}
		return fmt.Sprintf("(&%02X)", bytes[1])
	default:
		return "UNKNOWN ADDRESS MODE"
	}
}

func (d *Disassembler) lookupVar(val uint) (name string, ok bool) {
	for varn, varv := range d.vars {
		if varv.Ival == val {
			return varn, true
		}
	}

	return "", false
}

func (d *Disassembler) findBranchTargets() {
	// Track all reachable instructions. That is the address of the first
	// opcode of each instruction starting at offset and moving forwards.
	iloc := make(map[uint]bool)

	d.branchTargets = make(map[uint]int)

	d.walk(vtCode, func(cursor uint, _ int, b byte, op Opcode, opOk bool) int {
		iloc[cursor+d.BranchAdjust] = true // Reachable instruction
		if opOk {
			instruction := d.Program[cursor : cursor+op.Length]
			switch op.branchOrJump() {
			case btBranch:
				// This is ugly but it will do for now
				boff := int(instruction[1]) // All branches are 2 bytes long
				if op.Length == 3 {
					boff = int(instruction[2])
				}
				if boff > 127 {
					boff = boff - 256
				}
				// Adjust d.Offset to account for the 2 byte behavior, see
				// genBranch().
				boff += int(op.Length)

				tgt := cursor + uint(boff) + d.BranchAdjust
				if _, ok := d.branchTargets[tgt]; !ok {
					d.branchTargets[tgt] = 0 // value will be filled out later
				}
			case btJump:
				// Skip indirect jump since we don't know the target of the jump
				if b != OpJMPIndirect {
					tgt := (uint(instruction[2]) << 8) + uint(instruction[1])
					if _, ok := d.branchTargets[tgt]; !ok {
						d.branchTargets[tgt] = 0 // value will be filled out later
					}

					// If the jump target is a well known OS call then mark as seen
					if _, ok := addressToOsCallName[tgt]; ok {
						d.usedOSAddress[tgt] = true
					}
				}
			case btNeither:
				// Check instructions with Absolute addressing
				if op.AddrMode == Absolute {
					tgt := (uint(instruction[2]) << 8) + uint(instruction[1])
					if _, ok := osVectorAddresses[tgt]; ok {
						d.usedOSVector[tgt] = true
					}
				}
			}

			return len(instruction)
		}

		return 1
	})

	// Reject branch targets that point to unreachable instructions. This can
	// happen disassembling data and the byte values generate a branch
	// instruction with a relative address that does not point to the beginning
	// of a reachable instruction.
	for k := range d.branchTargets {
		if _, ok := iloc[k]; !ok {
			delete(d.branchTargets, k)
		}
	}

	// Sort branch targets in order of increasing address
	bt := make([]int, len(d.branchTargets))
	i := 0
	for k := range d.branchTargets {
		bt[i] = int(k)
		i++
	}
	sort.Ints(bt)
	for i, v := range bt {
		d.branchTargets[uint(v)] = i
	}
}

var disasmHeader = `\ ******************************************************************************
\
\ This disassembly was produced by bbcdisasm
\
\ ******************************************************************************

{{ if .UsedOSAddress }}\ OS Call Addresses
{{ $os := .OSAddress }}
{{- range $addr, $elem := .UsedOSAddress }}{{ printf "%-6s" (index $os $addr) }} = {{ printf "&%0X" $addr }}
{{ end }}
{{- end }}
{{ if .UsedOSVector }}\ OS Vector Addresses
{{ $vec := .OSVector }}
{{- range $addr, $elem := .UsedOSVector }}{{ printf "%-5s" (index $vec $addr) }} = {{ printf "&%0X" $addr }}
{{ end }}
{{- end }}
{{ if .Vars }}\ Defined Variables
{{ $var := .Vars }}
{{- range $name, $value := .Vars }}{{ printf "%-5s" $name }} = {{ print $value.Sval }}
{{ end }}
{{- end }}
{{ if .LoadAddr }}CODE% = {{ printf "&%X" .LoadAddr }}

ORG CODE%
{{ else -}}
{{ end }}
`
