package pulumi

import (
	"context"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/pkg/v3/backend"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) createStackIfNotExists(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	s, err := p.selectStack(ctx, cfg, stack)
	if s != nil {
		p.logger.Debug(ctx, "found stack %q, not going to create", p.stackRef.FullyQualifiedName().String())
		return nil
	} else if p.stackRef != nil {
		p.logger.Debug(ctx, "stack %q not found, creating...", p.stackRef.FullyQualifiedName().String())
		s, err = p.backend.CreateStack(ctx, p.stackRef, "", nil)
		if err != nil {
			return errors.Wrapf(err, "failed to create stack %q", p.stackRef.FullyQualifiedName().String())
		} else if s != nil {
			p.logger.Info(ctx, "created stack %q", s.Ref().FullyQualifiedName().String())
		}
	}
	return err
}

func (p *pulumi) selectStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) (backend.Stack, error) {
	err := p.login(ctx, cfg, stack)
	if err != nil {
		return nil, err
	}
	if s, err := p.backend.GetStack(ctx, p.stackRef); err != nil {
		return s, errors.Wrapf(err, "failed to get stack %q", p.stackRef)
	} else {
		return s, nil
	}
}
