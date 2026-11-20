# Bytecode format

`go-mrbc` emits the mruby **RITE 04.00** binary format used by mruby 4.0.0.
This document describes the layout implemented by the `bytecode` package.

All multi-byte integers are **big-endian** unless stated otherwise.

## File layout

```
+-------------------------------+
| RITE binary header (20 bytes) |
+-------------------------------+
| IREP section                  |
+-------------------------------+
| DBG section (optional)        |
+-------------------------------+
| LVAR section (optional)       |
+-------------------------------+
| END footer (8 bytes)          |
+-------------------------------+
```

The initial implementation emits only the binary header, the `IREP` section,
and the footer. mruby executes binaries without the optional sections.

## Binary header

| Offset | Size | Field | Value |
| --- | --- | --- | --- |
| 0 | 4 | identifier | `RITE` |
| 4 | 2 | major version | `04` |
| 6 | 2 | minor version | `00` |
| 8 | 4 | total binary size | uint32 |
| 12 | 4 | compiler name | `MATZ` |
| 16 | 4 | compiler version | `0000` |

`binary size` is the size of the whole file, including the header and footer.

## IREP section

| Field | Size | Notes |
| --- | --- | --- |
| section identifier | 4 | `IREP` |
| section size | 4 | includes the section header and all irep records |
| rite version | 4 | `0400` |

The section size covers the 12-byte section header plus the recursively
serialized irep records.

## IREP record

An irep record consists of a header, the instruction sequence block, the
literal pool, the symbol table, and zero or more child irep records.

### Record header

| Field | Size | Notes |
| --- | --- | --- |
| record size | 4 | size of this record's own header, iseq, pool, and symbols (not children) |
| nlocals | 2 | number of local slots, including `R0` (`self`) |
| nregs | 2 | number of registers required by this frame |
| rlen | 2 | number of child ireps that follow this record's symbol table |

### Instruction sequence block

| Field | Size | Notes |
| --- | --- | --- |
| clen | 2 | number of catch handlers |
| ilen | 4 | number of bytes of instruction code |
| iseq | ilen | raw instruction bytes |
| catch handlers | clen * 13 | appended after the instruction bytes |

A catch handler is:

| Field | Size | Notes |
| --- | --- | --- |
| type | 1 | 0 = rescue, 1 = ensure |
| begin | 4 | absolute byte offset into `iseq`, inclusive |
| end | 4 | absolute byte offset into `iseq`, exclusive |
| target | 4 | absolute byte offset into `iseq` |

Catch handlers are emitted for `rescue` and `ensure` constructs. `clen` is zero
when no handler is present.

### Literal pool

| Field | Size | Notes |
| --- | --- | --- |
| plen | 2 | number of pool entries |

Each entry starts with a one-byte type:

| Type | Name | Payload |
| --- | --- | --- |
| 0 | string | uint16 length, UTF-8 bytes, terminating NUL |
| 1 | int32 | uint32 |
| 3 | int64 | high uint32, low uint32 |
| 5 | float | IEEE 754 binary64, little-endian |
| 7 | bigint | length byte, sign byte, magnitude bytes |

Integers that fit in signed 32 bits are normally emitted as `int32`.
Floats are always written as 8 bytes in little-endian order, independent of
the host architecture.

### Symbol table

| Field | Size | Notes |
| --- | --- | --- |
| slen | 2 | number of symbols |

Each symbol is:

| Field | Size | Notes |
| --- | --- | --- |
| length | 2 | `0xFFFF` means a null symbol |
| bytes | length | symbol name |
| terminator | 1 | NUL |

### Child ireps

Child ireps are serialized immediately after the parent's symbol table, in the
order referenced by `BLOCK`, `LAMBDA`, `METHOD`, `EXEC`, `TDEF`, `SDEF`, and
related instructions. `rlen` gives their count.

## DBG section

The optional `DBG` section carries source line information. It is emitted when
the compiler is asked for debug information (`gomrbc -g`).

```
section identifier : "DBG\0"
section size       : uint32
filename count     : uint16
filenames          : count * (uint16 length, bytes)
debug records      : one per irep, in preorder
```

A debug record is:

| Field | Size | Notes |
| --- | --- | --- |
| record size | 4 | includes this field |
| file count | 2 | number of file entries |
| start position | 4 | instruction offset where the file starts |
| filename index | 2 | index into the filename table |
| line entry count | 4 | bytes of line data |
| line type | 1 | 2 = packed map |
| line data | variable | packed varint pairs |

The compiler emits one file entry per irep with `start position = 0`. The
packed map is a sequence of varint pairs `(pc delta, line delta)` starting at
`pc = 0, line = 0`; values accumulate. mruby resolves a program counter by
walking the pairs until the accumulated position exceeds the pc. This is the
format mruby 4.0's `mrb_debug_get_line` reads.

## LVAR section

The optional `LVAR` section stores local variable names. It is emitted unless
`--remove-lv` is used.

```
section identifier : "LVAR"
section size       : uint32
symbol count       : uint32
symbols            : count * (uint16 length, bytes)
records            : one per irep, in preorder
```

Each irep contributes `nlocals - 1` uint16 entries (registers `R1` through
`R[nlocals-1]`, excluding `R0`). An entry is an index into the symbol table, or
`0xFFFF` when the register has no name (for example a hidden block or keyword
slot).

Sections are written in the order `IREP`, `DBG`, `LVAR`, `END`.

## Instructions

An instruction is a one-byte opcode followed by its operands. Operand widths
are defined by `mruby/ops.h`:

| Code | Operands |
| --- | --- |
| `Z` | none |
| `B` | 8-bit |
| `BB` | two 8-bit |
| `BBB` | three 8-bit |
| `BS` | 8-bit, 16-bit |
| `BSS` | 8-bit, 16-bit, 16-bit |
| `S` | 16-bit |
| `W` | 24-bit |

Wide operands are big-endian.

### Extended operands

mruby reserves three prefix opcodes for operands that do not fit in 8 bits:

| Prefix | Effect |
| --- | --- |
| `EXT1` | first operand becomes 16-bit |
| `EXT2` | second operand becomes 16-bit |
| `EXT3` | first and second operands become 16-bit |

The prefix is followed by the real opcode and then the widened operands.
Third operands are always 8-bit.

### Jumps

Jump offsets are signed 16-bit values relative to the byte immediately after
the jump's operands. A jump instruction occupies three bytes (`JMP`) or four
bytes (`JMPIF`, `JMPNOT`, `JMPNIL`).

### Argument setup

Method, block, and lambda instruction sequences begin with `OP_ENTER`, whose
24-bit operand is an argument spec:

```
noblock:1 req:5 opt:5 rest:1 post:5 key:5 kdict:1 block:1
```

The VM uses this to place arguments into a fixed register layout:

```
R0              self
R1 ..           required positional arguments
then            optional positional arguments
then            rest argument (Array)
then            post arguments
then            keyword arguments
then            keyword dictionary (when declared)
then            block argument
then            other locals
```

### Sending messages

`OP_SEND`, `OP_SSEND`, `OP_SENDB`, and `OP_SSENDB` carry the method symbol and a
packed operand:

```
low nibble      number of positional arguments
high nibble     number of keyword arguments (15 means a keyword hash)
```

Arguments follow the receiver in consecutive registers. For the `*B` variants,
the block argument follows the positional and keyword arguments.

## Top-level irep

The top-level irep has no `OP_ENTER`. It ends with `RETURN` followed by `STOP`.
Method and block ireps end with a return instruction and do not include
`STOP`.

## Version policy

`go-mrbc` targets RITE 04.00 / mruby 4.0.0 exclusively. Version 1.0 of this
project will not produce or accept RITE 03xx binaries. The version constants
are centralized in the `bytecode` package so a future target can be added
deliberately rather than accidentally.
