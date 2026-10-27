package rename

import (
	"context"
	"fmt"
	"path/filepath"
)

// Apply performs the renames in order and returns the number completed. On
// error the renames already applied are left in place; the returned count says
// how far the plan got.
//
// A rename never replaces an existing entry, and never resolves outside Root,
// even if the tree changed after planning.
//
// onRename, when non-nil, is called after each rename succeeds, so a caller can
// report progress that reflects what actually happened.
func (p Plan) Apply(ctx context.Context, onRename func(Rename)) (int, error) {
	if len(p.Renames) == 0 {
		return 0, nil
	}
	target, err := openTargetDir(p.Root)
	if err != nil {
		return 0, fmt.Errorf("open target directory: %w", err)
	}
	//nolint:errcheck // read-only directory handles; closing them cannot lose data
	defer target.close()

	for i, r := range p.Renames {
		if err := ctx.Err(); err != nil {
			return i, err
		}
		if err := p.renameOne(target, r); err != nil {
			return i, fmt.Errorf("rename %s to %s: %w", r.From, filepath.Base(r.To), err)
		}
		if onRename != nil {
			onRename(r)
		}
	}
	return len(p.Renames), nil
}

func (p Plan) renameOne(target *targetDir, r Rename) error {
	dir, err := filepath.Rel(p.Root, filepath.Dir(r.From))
	if err != nil {
		return err
	}
	return target.rename(dir, filepath.Base(r.From), filepath.Base(r.To))
}
