package cxparsering

import (
	"bytes"

	cxpartialparsing "github.com/skycoin/skycoin-cx/n019_cxpartialparsing"
	cxparsingcompletor "github.com/skycoin/skycoin-cx/n020_cxparsingcompletor"
)

/*
passone perform cxpartial parsing using partialparsing.y and partialparsing.go
*/
func Passone(source string) int {

	parseErrors := cxpartialparsing.Parse(source)

	return parseErrors
}

/*
passtwo perform cxparsingcompletor parsing using parsingcompletor.y and parsingcompletor.go
*/

func Passtwo(b *bytes.Buffer) int {

	parseErrors := cxparsingcompletor.Parse(cxparsingcompletor.NewLexer(b))

	return parseErrors
}
