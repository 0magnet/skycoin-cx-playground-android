package parsingcompletor

import (
	"github.com/skycoin/skycoin-cx/n009_opcodes"
	"github.com/skycoin/skycoin-cx/n011_actions"
	cxinit "github.com/skycoin/skycoin-cx/n018_init"
)

var Initialized bool

func init() {
	InitCXCore()
}

func InitCXCore() {
	if !Initialized {
		if actions.AST == nil {
			actions.AST = cxinit.MakeProgram()
		}
		opcodes.RegisterOpcodes(actions.AST)

		Initialized = true
	}
}
