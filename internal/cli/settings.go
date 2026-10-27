package cli

import (
	"github.com/ruandre/casemv/internal/casing"
	"github.com/ruandre/casemv/internal/rename"
)

// settings is the validated form of the command line and config file.
type settings struct {
	convention     casing.Convention
	target         string
	recursive      bool
	dirs           bool
	hidden         bool
	exclude        []string
	dryRun         bool
	quiet          bool
	lowerExtension bool
	text           bool
}

// namer gives each entry its new name: the convention, lowering extensions if asked.
//
//nolint:ireturn // chooses between Namer implementations by setting
func (s settings) namer() rename.Namer {
	if s.lowerExtension {
		return casing.LowerExtension{Convention: s.convention}
	}
	return s.convention
}

func (s settings) scanOptions() rename.Options {
	return rename.Options{
		Root:          s.target,
		Recursive:     s.recursive,
		IncludeDirs:   s.dirs,
		IncludeHidden: s.hidden,
		Exclude:       s.exclude,
	}
}
