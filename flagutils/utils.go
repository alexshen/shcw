package flagutils

import (
	"flag"
	"strings"
)

// ListArg is an argument of a list of values. There are two ways of specifying
// multiple values for a ListArg, one is to provide values with comma separated
// values, the other is to provide flags more than once. Mixing two ways is
// possible.
//
// Examples:
//
//	-foo a,b
//	-foo a -foo b
//	-foo a,b -foo c
type ListArg []string

func (f *ListArg) String() string {
	return strings.Join(([]string)(*f), ",")
}

func (f *ListArg) Set(value string) error {
	*f = append(*f, strings.Split(value, ",")...)
	return nil
}

func (f *ListArg) Get() any {
	return *f
}

func List(name string, def string, usage string) *ListArg {
	var value ListArg
	if def != "" {
		value.Set(def)
	}
	flag.Var(&value, name, usage)
	return &value
}
