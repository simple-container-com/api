package pulumi

import (
	"context"

	"github.com/pkg/errors"

	"api/pkg/api"
)

func (p *pulumi) createStackIfNotExists(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	_, be, stackRef, err := p.login(ctx, cfg, stack)
	if err != nil {
		return err
	}
	if s, err := be.GetStack(ctx, stackRef); err != nil {
		return err
	} else if s != nil {
		p.logger.Debug(ctx, "found stack %q, not going to create", stackRef.String())
		return nil
	} else {
		p.logger.Debug(ctx, "stack %q not found, creating...", stackRef.String())
		s, err = be.CreateStack(ctx, stackRef, "", nil)
		if err != nil {
			return errors.Wrapf(err, "failed to create stack %q", stackRef.String())
		} else if s != nil {
			p.logger.Info(ctx, "created stack %q", s.Ref().String())
		}
	}
	return nil
}
