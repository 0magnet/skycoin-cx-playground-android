//go:build !ptr64

package ast

import "github.com/skycoin/skycoin-cx/n001_types"

// Pointer type, CXArg Constants
var ConstCxArg_PTR = Param(types.I32) // TODO:PTR use right type when we have ptr alias in cx
