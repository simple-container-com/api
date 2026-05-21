package pulumi

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"gocloud.dev/gcerrors"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) createStackIfNotExists(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	s, err := p.selectStack(ctx, cfg, stack)
	if s != nil {
		p.logger.Debug(ctx, "✅ Found stack %q, not going to create", p.stackRef.FullyQualifiedName().String())
		p.logger.Debug(ctx, "🔍 IMPORTANT: Stack found but resources may be missing due to state storage backend differences")
		p.logger.Debug(ctx, "🔍 If you see resources being 'created' instead of 'same', there's a state storage mismatch!")
		return nil
	} else if p.stackRef != nil && p.backend != nil {
		p.logger.Debug(ctx, "❌ Stack %q not found, creating...", p.stackRef.FullyQualifiedName().String())
		s, err = p.backend.CreateStack(ctx, p.stackRef, "", nil, nil)
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
	s, err := p.backend.GetStack(ctx, p.stackRef)
	if err != nil {
		// Treat "checkpoint blob not found" as "stack does not exist".
		//
		// Pulumi's diy backend (pkg/v3/backend/diy) is supposed to map a
		// missing checkpoint to (nil, nil) from GetStack — its own
		// errCheckpointNotFound sentinel handles that. But the path runs
		// through gocloud.dev/blob.Bucket.Exists, which only converts
		// provider errors to (false, nil) when gcerrors.Code(err) ==
		// gcerrors.NotFound.
		//
		// Recent transitive bumps to cloud.google.com/go/storage (and the
		// equivalent S3/Azure clients) sometimes surface a 404 through an
		// error path that gocloud no longer classifies as NotFound — the
		// error reaches Exists as code=Unknown, Exists returns (false,
		// wrapped-err) instead of (false, nil), stackExists wraps that
		// as "failed to load checkpoint", and GetStack returns the wrap
		// rather than the (nil, nil) "missing stack" contract that the
		// rest of SC's createStackIfNotExists / selectStack callers
		// depend on.
		//
		// This affected external SC consumers on 2026.5.31 (e.g. the
		// wize-rooms-api deploy on 2026-05-21) with:
		//   failed to get parent stack "wize-rooms-api":
		//   failed to get stack "wize-rooms-api":
		//   failed to load checkpoint: blob (key ".pulumi/stacks/<proj>/<stack>.json")
		//   (code=Unknown): storage: object doesn't exist:
		//   googleapi: Error 404: No such object: ...
		//
		// Restore the v3.184-era contract here: if the underlying error
		// is a NotFound (either by gocloud code or by the layered string
		// pattern that surfaces from current GCS/S3 clients), treat
		// GetStack as having returned (nil, nil) — the caller will then
		// CreateStack as it did before the regression.
		if stackCheckpointNotFound(err) {
			return nil, nil
		}
		return s, errors.Wrapf(err, "failed to get stack %q", p.stackRef)
	}
	return s, nil
}

// stackCheckpointNotFound returns true when err coming back from the diy
// backend's GetStack indicates that the underlying checkpoint blob is
// missing — i.e. the stack does not yet exist in state storage.
//
// First check is the structured one: gocloud's gcerrors.Code. That's what
// blob.Bucket.Exists uses internally to convert to (false, nil), and when
// it works we never hit this function in the first place — the structured
// path is the happy case we're patching around.
//
// Second check is a string match on the wrapped error message. We use
// it only as a fallback for the case where the underlying provider client
// (GCS / S3 / Azure) wraps the 404 in a way that gcerrors no longer sees
// as NotFound. We deliberately scope the match to error chains that
// originated in Pulumi's "failed to load checkpoint:" wrapper so we don't
// accidentally swallow unrelated NotFound-shaped errors from elsewhere
// in the deploy program.
func stackCheckpointNotFound(err error) bool {
	if err == nil {
		return false
	}
	if gcerrors.Code(err) == gcerrors.NotFound {
		return true
	}
	msg := err.Error()
	if !strings.Contains(msg, "failed to load checkpoint") {
		return false
	}
	// Provider-specific 404 markers that gcerrors.Code may miss after a
	// transitive bump. Match case-insensitively to defend against
	// formatting drift across client versions ("NotFound" vs "notFound",
	// "Not Found" with space, etc.):
	//   - GCS:   "object doesn't exist" / "notFound" / "Error 404"
	//   - S3 v1: "NoSuchKey"
	//   - S3 v2: "api error NotFound" / "StatusCode: 404"
	//   - Azure: "BlobNotFound" / "ResourceNotFound"
	//
	// The "404" suffix is intentional and load-bearing: it's the HTTP
	// status code that virtually every cloud-storage provider includes
	// in the wrapped error for a missing object, regardless of the
	// SDK's NotFound enum naming.
	msgLower := strings.ToLower(msg)
	for _, marker := range []string{
		"object doesn't exist",
		"notfound",
		"nosuchkey",
		"blobnotfound",
		"resourcenotfound",
		"statuscode: 404",
		"error 404",
	} {
		if strings.Contains(msgLower, marker) {
			return true
		}
	}
	return false
}
