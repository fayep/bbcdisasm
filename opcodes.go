package bbcdisasm

import (
	"fmt"
)

const labelFormatString = "label_%d"

// AddressingMode enumerates the different address modes of 6502 instructions
type AddressingMode int

// Addressing Modes
//  None        - no addressing mode                          - BRK
//  Accumulator - uses the accumulator register               - ASL A
//  Immediate   - using a data constant                       - LDA #FF
//  Absolute    - using a fixed address                       - LDA &1234
//  ZeroPage    - using a fixed zero page address             - LDA &12
//  ZeroPageX   - using zero page address+X                   - LDA &12,X
//  ZeroPageY   - using zero page address+Y (LDX only)        - LDX &12,Y
//  ZeroPageRel - using zero page address+immediate value     - BBR6 &12, #34
//  Indirect    - using an address stored in memory           - LDA (&1234)
//  AbsoluteX   - using an absolute address+X                 - LDA &1234,X
//  AbsoluteY   - using an absolute address+Y                 - LDA &1234,Y
//  IndirectX   - a table of zero page addresses indexed by X - LDA (&80,X)
//  IndirectY   - a table of zero page addresses indexed by Y - LDA (&80,Y)
//  IndirectZP  - An indirect zero page address
const (
	None AddressingMode = iota
	Accumulator
	Immediate
	Absolute
	ZeroPage
	ZeroPageX
	ZeroPageY
	ZeroPageRel
	Indirect
	AbsoluteX
	AbsoluteY
	IndirectX
	IndirectY
	IndirectZP
)

// Opcode defines a 6502 opcode
type Opcode struct {
	Value    byte   // Byte value for the opcode. All opcodes are one byte long.
	Name     string // Human readable instruction 'name'
	Length   uint   // Num bytes for instruction and arguments, includes opcode
	AddrMode AddressingMode
}

// TODO - Constants for all instructions?
const (
	OpJMPAbsolute = 0x4C
	OpJMPIndirect = 0x6C
	OpJSRAbsolute = 0x20
)

var (
	// OpCodes defines the documented instructions of the 6502 CPU. It also
	// includes a couple of undocumented instructions.
	// Most opcodes from http://www.6502.org/tutorials/6502opcodes.html
	// ANC, SLO, SRE from https://github.com/mattgodbolt/jsbeeb/blob/master/6502.opcodes.js
	OpCodes = []Opcode{
		{0x69, "ADC", 2, Immediate},
		{0x65, "ADC", 2, ZeroPage},
		{0x75, "ADC", 2, ZeroPageX},
		{0x6D, "ADC", 3, Absolute},
		{0x7D, "ADC", 3, AbsoluteX},
		{0x79, "ADC", 3, AbsoluteY},
		{0x61, "ADC", 2, IndirectX},
		{0x71, "ADC", 2, IndirectY},
		{0x72, "ADC", 2, IndirectZP},

		{0x0B, "ANC", 2, Immediate},
		{0x2B, "ANC", 2, Immediate},

		{0x29, "AND", 2, Immediate},
		{0x25, "AND", 2, ZeroPage},
		{0x35, "AND", 2, ZeroPageX},
		{0x2D, "AND", 3, Absolute},
		{0x3D, "AND", 3, AbsoluteX},
		{0x39, "AND", 3, AbsoluteY},
		{0x21, "AND", 2, IndirectX},
		{0x31, "AND", 2, IndirectY},
		{0x32, "AND", 2, IndirectZP},

		{0x0A, "ASL", 1, Accumulator},
		{0x06, "ASL", 2, ZeroPage},
		{0x16, "ASL", 2, ZeroPageX},
		{0x0E, "ASL", 3, Absolute},
		{0x1E, "ASL", 3, AbsoluteX},

		{0x0F, "BBR0", 3, None},
		{0x1F, "BBR1", 3, None},
		{0x2F, "BBR2", 3, None},
		{0x3F, "BBR3", 3, None},
		{0x4F, "BBR4", 3, None},
		{0x5F, "BBR5", 3, None},
		{0x6F, "BBR6", 3, None},
		{0x7F, "BBR7", 3, None},
		{0x8F, "BBS0", 3, None},
		{0x9F, "BBS1", 3, None},
		{0xAF, "BBS2", 3, None},
		{0xBF, "BBS3", 3, None},
		{0xCF, "BBS4", 3, None},
		{0xDF, "BBS5", 3, None},
		{0xEF, "BBS6", 3, None},
		{0xFF, "BBS7", 3, None},

		{0x24, "BIT", 2, ZeroPage},
		{0x2C, "BIT", 3, Absolute},

		{0x10, "BPL", 2, None}, // all the branch instructions have special cased
		{0x30, "BMI", 2, None}, // printing
		{0x50, "BVC", 2, None},
		{0x70, "BVS", 2, None},
		{0x80, "BRA", 2, None},
		{0x90, "BCC", 2, None},
		{0xB0, "BCS", 2, None},
		{0xD0, "BNE", 2, None},
		{0xF0, "BEQ", 2, None},

		{0x00, "BRK", 1, None},

		{0xC9, "CMP", 2, Immediate},
		{0xC5, "CMP", 2, ZeroPage},
		{0xD5, "CMP", 2, ZeroPageX},
		{0xCD, "CMP", 3, Absolute},
		{0xDD, "CMP", 3, AbsoluteX},
		{0xD9, "CMP", 3, AbsoluteY},
		{0xC1, "CMP", 2, IndirectX},
		{0xD1, "CMP", 2, IndirectY},

		{0xE0, "CPX", 2, Immediate},
		{0xE4, "CPX", 2, ZeroPage},
		{0xEC, "CPX", 3, Absolute},

		{0xC0, "CPY", 2, Immediate},
		{0xC4, "CPY", 2, ZeroPage},
		{0xCC, "CPY", 3, Absolute},

		{0xC6, "DEC", 2, ZeroPage},
		{0xD6, "DEC", 2, ZeroPageX},
		{0xCE, "DEC", 3, Absolute},
		{0xDE, "DEC", 3, AbsoluteX},

		{0x49, "EOR", 2, Immediate},
		{0x45, "EOR", 2, ZeroPage},
		{0x55, "EOR", 2, ZeroPageX},
		{0x4D, "EOR", 3, Absolute},
		{0x5D, "EOR", 3, AbsoluteX},
		{0x59, "EOR", 3, AbsoluteY},
		{0x41, "EOR", 2, IndirectX},
		{0x51, "EOR", 2, IndirectY},

		{0x18, "CLC", 1, None},
		{0x38, "SEC", 1, None},
		{0x58, "CLI", 1, None},
		{0x78, "SEI", 1, None},
		{0xB8, "CLV", 1, None},
		{0xD8, "CLD", 1, None},
		{0xF8, "SED", 1, None},

		{0xE6, "INC", 2, ZeroPage},
		{0xF6, "INC", 2, ZeroPageX},
		{0xEE, "INC", 3, Absolute},
		{0xFE, "INC", 3, AbsoluteX},

		{OpJMPAbsolute, "JMP", 3, Absolute}, // special cased when printing
		{OpJMPIndirect, "JMP", 3, Indirect},

		{OpJSRAbsolute, "JSR", 3, Absolute}, // special cased when printing

		{0xA9, "LDA", 2, Immediate},
		{0xA5, "LDA", 2, ZeroPage},
		{0xB5, "LDA", 2, ZeroPageX},
		{0xAD, "LDA", 3, Absolute},
		{0xBD, "LDA", 3, AbsoluteX},
		{0xB9, "LDA", 3, AbsoluteY},
		{0xA1, "LDA", 2, IndirectX},
		{0xB1, "LDA", 2, IndirectY},

		{0xA2, "LDX", 2, Immediate},
		{0xA6, "LDX", 2, ZeroPage},
		{0xB6, "LDX", 2, ZeroPageY},
		{0xAE, "LDX", 3, Absolute},
		{0xBE, "LDX", 3, AbsoluteY},

		{0xA0, "LDY", 2, Immediate},
		{0xA4, "LDY", 2, ZeroPage},
		{0xB4, "LDY", 2, ZeroPageX},
		{0xAC, "LDY", 3, Absolute},
		{0xBC, "LDY", 3, AbsoluteX},

		{0x4A, "LSR", 1, Accumulator},
		{0x46, "LSR", 2, ZeroPage},
		{0x56, "LSR", 2, ZeroPageX},
		{0x4E, "LSR", 3, Absolute},
		{0x5E, "LSR", 3, AbsoluteX},

		{0xEA, "NOP", 1, None},

		{0x09, "ORA", 2, Immediate},
		{0x05, "ORA", 2, ZeroPage},
		{0x15, "ORA", 2, ZeroPageX},
		{0x0D, "ORA", 3, Absolute},
		{0x1D, "ORA", 3, AbsoluteX},
		{0x19, "ORA", 3, AbsoluteY},
		{0x01, "ORA", 2, IndirectX},
		{0x11, "ORA", 2, IndirectY},

		{0xAA, "TAX", 1, None},
		{0x8A, "TXA", 1, None},
		{0xCA, "DEX", 1, None},
		{0xE8, "INX", 1, None},
		{0xA8, "TAY", 1, None},
		{0x98, "TYA", 1, None},
		{0x88, "DEY", 1, None},
		{0xC8, "INY", 1, None},

		{0x07, "RMB0", 2, ZeroPage},
		{0x17, "RMB1", 2, ZeroPage},
		{0x27, "RMB2", 2, ZeroPage},
		{0x37, "RMB3", 2, ZeroPage},
		{0x47, "RMB4", 2, ZeroPage},
		{0x57, "RMB5", 2, ZeroPage},
		{0x67, "RMB6", 2, ZeroPage},
		{0x77, "RMB7", 2, ZeroPage},


		{0x2A, "ROL", 1, Accumulator},
		{0x26, "ROL", 2, ZeroPage},
		{0x36, "ROL", 2, ZeroPageX},
		{0x2E, "ROL", 3, Absolute},
		{0x3E, "ROL", 3, AbsoluteX},

		{0x6A, "ROR", 1, Accumulator},
		{0x66, "ROR", 2, ZeroPage},
		{0x76, "ROR", 2, ZeroPageX},
		{0x6E, "ROR", 3, Absolute},
		{0x7E, "ROR", 3, AbsoluteX},

		{0x40, "RTI", 1, None},

		{0x60, "RTS", 1, None},

		{0xE9, "SBC", 2, Immediate},
		{0xE5, "SBC", 2, ZeroPage},
		{0xF5, "SBC", 2, ZeroPageX},
		{0xED, "SBC", 3, Absolute},
		{0xFD, "SBC", 3, AbsoluteX},
		{0xF9, "SBC", 3, AbsoluteY},
		{0xE1, "SBC", 2, IndirectX},
		{0xF1, "SBC", 2, IndirectY},

		{0x87, "SMB0", 2, ZeroPage},
		{0x97, "SMB1", 2, ZeroPage},
		{0xA7, "SMB2", 2, ZeroPage},
		{0xB7, "SMB3", 2, ZeroPage},
		{0xC7, "SMB4", 2, ZeroPage},
		{0xD7, "SMB5", 2, ZeroPage},
		{0xE7, "SMB6", 2, ZeroPage},
		{0xF7, "SMB7", 2, ZeroPage},

/*
		{0x47, "SRE", 2, ZeroPage},
		{0x57, "SRE", 2, ZeroPageX},
		{0x4F, "SRE", 3, Absolute},
		{0x5F, "SRE", 3, AbsoluteX},
		{0x5B, "SRE", 3, AbsoluteY},
		{0x43, "SRE", 2, IndirectX},
		{0x53, "SRE", 2, IndirectY},
*/

		{0x85, "STA", 2, ZeroPage},
		{0x95, "STA", 2, ZeroPageX},
		{0x8D, "STA", 3, Absolute},
		{0x9D, "STA", 3, AbsoluteX},
		{0x99, "STA", 3, AbsoluteY},
		{0x81, "STA", 2, IndirectX},
		{0x91, "STA", 2, IndirectY},

		{0x9A, "TXS", 1, None},
		{0xBA, "TSX", 1, None},
		{0x48, "PHA", 1, None},
		{0x68, "PLA", 1, None},
		{0x08, "PHP", 1, None},
		{0x28, "PLP", 1, None},

/*
		{0x07, "SLO", 2, ZeroPage},
		{0x17, "SLO", 2, ZeroPageX},
		{0x0F, "SLO", 3, Absolute},
		{0x1F, "SLO", 3, AbsoluteX},
		{0x1B, "SLO", 3, AbsoluteY},
		{0x03, "SLO", 2, IndirectX},
		{0x13, "SLO", 2, IndirectY},
*/

		{0x86, "STX", 2, ZeroPage},
		{0x96, "STX", 2, ZeroPageY},
		{0x8E, "STX", 3, Absolute},

		{0x84, "STY", 2, ZeroPage},
		{0x94, "STY", 2, ZeroPageX},
		{0x8C, "STY", 3, Absolute},
	}

	// OpCodesMap maps from opcode byte value to Opcode. Initialized by init()
	OpCodesMap map[byte]Opcode

	// UndocumentedInstructions is not exhaustive and only tracks the opcodes
	// that are included in OpCodesMap.
	UndocumentedInstructions = []string{"ANC", "SRE", "SLO"}

	branchInstructions = []string{"BPL", "BMI", "BVC", "BRA", "BVS", "BCC", "BCS", "BNE", "BEQ", "BBR0", "BBR1", "BBR2", "BBR3", "BBR4", "BBR5", "BBR6", "BBR7", "BBS0", "BBS1", "BBS2", "BBS3", "BBS4", "BBS5", "BBS6", "BBS7"}

	jumpInstructions = []string{"JMP", "JSR"}

	// Maps absolute addresses to names of BBC MICRO OS calls
	addressToOsCallName = map[uint]string{
		0xFFB9: "OSRDRM",
		0xFFBF: "OSEVEN",
		0xFFC2: "GSINIT",
		0xFFC5: "GSREAD",
		0xFFC8: "NVRDCH", // non-vectored OSRDCH
		0xFFCB: "NVWRCH", // non-vectored OSWRCH
		0xFFCE: "OSFIND",
		0xFFE0: "OSRDCH",
		0xFFE3: "OSASCI",
		0xFFE7: "OSNEWL",
		0xFFEE: "OSWRCH",
		0xFFF1: "OSWORD",
		0xFFF4: "OSBYTE",
		0xFFF7: "OSCLI",
	}

	// Maps OS Vector Addresses to string identifiers
	osVectorAddresses = map[uint]string{
		0x200: "USERV",
		0x202: "BRKV",
		0x204: "IRQ1V",
		0x206: "IRQ2V",
		0x208: "CLIV",
		0x20A: "BYTEV",
		0x20C: "WORDV",
		0x20E: "WRCHV",
		0x210: "RDCHV",
		0x212: "FILEV",
		0x214: "ARGV",
		0x216: "BGETV",
		0x218: "BPUTV",
		0x21A: "GBPBV",
		0x21C: "FINDV",
		0x21E: "FSCV",
		0x220: "EVENTV",
		0x222: "UPTV",
		0x224: "NETV",
		0x226: "VDUV",
		0x228: "KEYV",
		0x22A: "INSV",
		0x22C: "REMV",
		0x22E: "CNPV",
		0x230: "IND1V", // Not documented in BBC Micro AUG
		0x232: "IND2V",
		0x234: "IND3V",
	}
)

type branchType int

const (
	btNeither branchType = iota
	btBranch
	btJump
)

func init() {
	OpCodesMap = make(map[byte]Opcode)
	for _, op := range OpCodes {
		OpCodesMap[op.Value] = op
	}
}

func (o *Opcode) branchOrJump() branchType {
	for _, v := range branchInstructions {
		if o.Name == v {
			return btBranch
		}
	}

	for _, v := range jumpInstructions {
		if o.Name == v {
			return btJump
		}
	}

	return btNeither
}

func genAbsoluteOsCall(bytes []byte, branchTargets map[uint]int) string {
	addr := (uint(bytes[2]) << 8) + uint(bytes[1])

	// Check if it is a well known OS address
	if osCall, ok := addressToOsCallName[addr]; ok {
		return osCall
	}

	// Check if it is a known branch target
	if tgtIdx, ok := branchTargets[addr]; ok {
		return fmt.Sprintf(labelFormatString, tgtIdx)
	}

	return fmt.Sprintf("&%04X", addr)
}

func genBranch(bytes []byte, cursor, branchAdjust uint, branchTargets map[uint]int, length uint) string {
	// From http://www.6502.org/tutorials/6502opcodes.html
	// "When calculating branches a forward branch of 6 skips the following 6
	// bytes so, effectively the program counter points to the address that is 8
	// bytes beyond the address of the branch opcode; and a backward branch of
	// $FA (256-6) goes to an address 4 bytes before the branch instruction."
	boff := int(bytes[1]) // All branches are 2 bytes long
	if length == 3 {
		boff = int(bytes[2])
	}

	if boff > 127 {
		boff = boff - 256
	}
	// Adjust offset to account for the 2 byte behavior from the comment block
	// above.
	boff += int(length)

	tgt := cursor + uint(boff) + branchAdjust
	// TODO: Explore branch relative offset in the end of line comment

	tgtIdx, ok := branchTargets[tgt]
	if !ok {
		// If the branch offset is not a 'reachable' instruction then express
		// the branch with the relative offset. However beebasm interprets an
		// integer literal as an absolute address, so instead write out an
		// expression that generates the same opcodes, e.g. P%+12 or P%-87
		if length == 3 {
			return fmt.Sprintf("&%02X,P%%%+d", bytes[1], boff)
		}
		return fmt.Sprintf("P%%%+d", boff)
	}
	if length == 3 {
		return fmt.Sprintf("&%02X,"+labelFormatString, bytes[1], tgtIdx)
	}
	return fmt.Sprintf(labelFormatString, tgtIdx)
}
