package compiler

import (
	"bytes"
	"encoding/binary"
	//"fmt"
	"github.com/go-interpreter/wagon/disasm"
	"github.com/go-interpreter/wagon/wasm"
	"github.com/go-interpreter/wagon/validate"
	"github.com/perlin-network/life/compiler/opcodes"
	"github.com/perlin-network/life/utils"
)

type Module struct {
	Base *wasm.Module
}

type InterpreterCode struct {
	NumRegs    int
	NumParams  int
	NumLocals  int
	NumReturns int
	Bytes      []byte
}

func LoadModule(raw []byte) (*Module, error) {
	reader := bytes.NewReader(raw)

	m, err := wasm.ReadModule(reader, nil)
	if err != nil {
		return nil, err
	}

	err = validate.VerifyModule(m)
	if err != nil {
		return nil, err
	}

	return &Module{
		Base: m,
	}, nil
}

func (m *Module) CompileForInterpreter() (_retCode []InterpreterCode, retErr error) {
	defer utils.CatchPanic(&retErr)

	ret := make([]InterpreterCode, 0)

	if m.Base.Import != nil {
		for i := 0; i < len(m.Base.Import.Entries); i++ {
			e := &m.Base.Import.Entries[i]
			if e.Kind != wasm.ExternalFunction {
				continue
			}
			tyID := e.Type.(wasm.FuncImport).Type
			ty := &m.Base.Types.Entries[int(tyID)]

			buf := &bytes.Buffer{}

			binary.Write(buf, binary.LittleEndian, uint32(1)) // value ID
			binary.Write(buf, binary.LittleEndian, opcodes.InvokeImport)
			binary.Write(buf, binary.LittleEndian, uint32(i))

			binary.Write(buf, binary.LittleEndian, uint32(0))
			if len(ty.ReturnTypes) != 0 {
				binary.Write(buf, binary.LittleEndian, opcodes.ReturnValue)
				binary.Write(buf, binary.LittleEndian, uint32(1))
			} else {
				binary.Write(buf, binary.LittleEndian, opcodes.ReturnVoid)
			}

			code := buf.Bytes()

			ret = append(ret, InterpreterCode{
				NumRegs:    2,
				NumParams:  len(ty.ParamTypes),
				NumLocals:  0,
				NumReturns: len(ty.ReturnTypes),
				Bytes:      code,
			})
		}
	}

	numFuncImports := len(ret)
	ret = append(ret, make([]InterpreterCode, len(m.Base.FunctionIndexSpace))...)

	for i, f := range m.Base.FunctionIndexSpace {
		//fmt.Printf("Compiling function %d (%+v) with %d locals\n", i, f.Sig, len(f.Body.Locals))
		d, err := disasm.Disassemble(f, m.Base)
		if err != nil {
			panic(err)
		}
		compiler := NewSSAFunctionCompiler(m.Base, d)
		compiler.CallIndexOffset = numFuncImports
		compiler.Compile()
		//fmt.Println(compiler.Code)
		//fmt.Printf("%+v\n", compiler.NewCFGraph())
		numRegs := compiler.RegAlloc()
		//fmt.Println(compiler.Code)
		numLocals := 0
		for _, v := range f.Body.Locals {
			numLocals += int(v.Count)
		}
		ret[numFuncImports+i] = InterpreterCode{
			NumRegs:    numRegs,
			NumParams:  len(f.Sig.ParamTypes),
			NumLocals:  numLocals,
			NumReturns: len(f.Sig.ReturnTypes),
			Bytes:      compiler.Serialize(),
		}
	}

	return ret, nil
}
