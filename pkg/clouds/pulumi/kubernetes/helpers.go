package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type deployChartCfg struct {
	name      string
	defaultNS string
	repo      *string
	version   *string
	values    map[string]any
}

// SanitizeK8sName sanitizes a name to comply with Kubernetes RFC 1123 label requirements.
// RFC 1123 labels must consist of lowercase alphanumeric characters or '-',
// and must start and end with an alphanumeric character.
// Regex: [a-z0-9]([-a-z0-9]*[a-z0-9])?
func SanitizeK8sName(name string) string {
	// Replace underscores with hyphens to comply with RFC 1123
	return strings.ReplaceAll(name, "_", "-")
}

// sanitizeK8sName is an internal helper that calls the exported function
func sanitizeK8sName(name string) string {
	return SanitizeK8sName(name)
}

func ensureNamespace(ctx *sdk.Context, input api.ResourceInput, params pApi.ProvisionParams, namespace string) (*corev1.Namespace, error) {
	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}
	sanitizedNamespace := sanitizeK8sName(namespace)
	return corev1.NewNamespace(ctx, fmt.Sprintf("create-ns-%s-%s", sanitizedNamespace, input.ToResName(input.Descriptor.Name)), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: sdk.String(sanitizedNamespace),
		},
	}, opts...)
}

func deployOperatorChart[T k8s.HelmOperatorChart](ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams, cfg deployChartCfg) (T, *helmv4.Chart, error) {
	chartCfg, ok := input.Descriptor.Config.Config.(T)
	if !ok {
		return chartCfg, nil, errors.Errorf("failed to convert chart's %q config for %q", cfg.name, input.Descriptor.Type)
	}

	params.Log.Info(ctx.Context(), "Deploying %q helm chart...", cfg.name)

	releaseName := input.ToResName(input.Descriptor.Name)
	namespace := input.ToResName(lo.If(cfg.defaultNS != "", cfg.defaultNS).Else("operators"))
	if chartCfg.OperatorNamespace() != nil {
		namespace = lo.FromPtr(chartCfg.OperatorNamespace())
	}

	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}
	ns, err := ensureNamespace(ctx, input, params, namespace)
	if err != nil {
		return chartCfg, nil, errors.Wrapf(err, "failed to create namespace %q", namespace)
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{ns}))

	values := lo.Assign(chartCfg.Values(), cfg.values)
	chart, err := helmv4.NewChart(ctx, releaseName, &helmv4.ChartArgs{
		Chart:   sdk.String(cfg.name),
		Name:    sdk.String(releaseName),
		Version: sdk.StringPtrFromPtr(cfg.version),
		// Atomic:          sdk.BoolPtr(true),
		RepositoryOpts: helmv4.RepositoryOptsArgs{
			Repo: sdk.StringPtrFromPtr(cfg.repo),
		},
		Namespace: sdk.String(namespace),
		Values:    sdk.ToMap(values),
	}, opts...)
	if err != nil {
		return chartCfg, nil, errors.Wrapf(err, "failed to install %q helm chart", cfg.name)
	}
	return chartCfg, chart, nil
}

func waitUntilSecretExists(ctx *sdk.Context, params pApi.ProvisionParams, secretName string, callback func(secret *corev1.Secret) (sdk.Output, error), opts ...sdk.ResourceOption) (sdk.Output, error) {
	withTimeout, cancel := context.WithTimeout(ctx.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(1 * time.Second)
	idx := 0
	for {
		select {
		case <-withTimeout.Done():
			params.Log.Error(ctx.Context(), "failed to wait until postgres instance secrets exist")
			return nil, errors.Errorf("timeout while waiting until postgres instance secret %q exists", secretName)
		case <-ticker.C:
			params.Log.Info(ctx.Context(), "waiting for secret %q to exist", secretName)
			secret, err := corev1.GetSecret(ctx, fmt.Sprintf("%s-get-%d", secretName, idx), sdk.ID(secretName), nil, opts...)
			idx++
			if err != nil {
				continue
			}
			time.Sleep(5 * time.Second) // wait until secret actually exists
			return callback(secret)
		}
	}
}

func exportSecretValues[T any](ctx *sdk.Context, secretName, exportName string, params pApi.ProvisionParams, object *T, opts ...sdk.ResourceOption) error {
	connectionParamsOut, err := waitUntilSecretExists(ctx, params, secretName, func(secret *corev1.Secret) (sdk.Output, error) {
		return secret.Data.ApplyT(func(data map[string]string) (any, error) {
			return decodeBase64FieldsToMapOutput(data, object)
		}), nil
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to wait until secret %q exist", secretName)
	}
	ctx.Export(exportName, connectionParamsOut)
	return nil
}

func readObjectFromStack[T any](ctx *sdk.Context, refName string, parentRef string, exportName string, obj *T, secret bool) (*T, error) {
	objMap, err := pApi.GetValueFromStack[map[string]any](ctx, refName, parentRef, exportName, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %q from parent stack", exportName)
	} else if len(objMap) == 0 {
		return nil, errors.Errorf("failed to get %q (empty) from parent stack", exportName)
	}
	res, err := mapToObject(objMap, obj)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal output %q from parent stack", exportName)
	}
	return res, nil
}

func mapToObject[T any](values map[string]any, object *T) (*T, error) {
	marshalledBytes, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(marshalledBytes, object)
	if err != nil {
		return nil, err
	}
	return object, nil
}

func objectToStringMapOutput[T any](object *T) (sdk.StringMapOutput, error) {
	mapOut := make(map[string]string)
	marshalledBytes, err := json.Marshal(object)
	if err != nil {
		return sdk.ToStringMapOutput(nil), err
	}
	err = json.Unmarshal(marshalledBytes, &mapOut)
	if err != nil {
		return sdk.ToStringMapOutput(nil), err
	}
	return sdk.ToStringMapOutput(lo.MapValues(mapOut, func(value string, key string) sdk.StringOutput {
		return sdk.String(value).ToStringOutput()
	})), nil
}

func decodeBase64FieldsToMapOutput[T any](fields map[string]string, object *T) (sdk.MapOutput, error) {
	mapOut := make(map[string]string)
	res, err := decodeBase64FieldsToObject(fields, object)
	if err != nil {
		return sdk.ToMapOutput(nil), err
	}
	marshalledBytes, err := json.Marshal(res)
	if err != nil {
		return sdk.ToMapOutput(nil), err
	}
	err = json.Unmarshal(marshalledBytes, &mapOut)
	if err != nil {
		return sdk.ToMapOutput(nil), err
	}
	return sdk.ToMapOutput(lo.MapValues(mapOut, func(value string, key string) sdk.Output {
		return sdk.String(value).ToStringOutput()
	})), nil
}

func decodeBase64FieldsToObject[T any](fields map[string]string, object *T) (*T, error) {
	decodedMap, err := decodeBase64Fields(fields)
	if err != nil {
		return nil, err
	}
	marshalledBytes, err := json.Marshal(decodedMap)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(marshalledBytes, object)
	if err != nil {
		return nil, err
	}
	return object, nil
}

func decodeBase64Fields(fields map[string]string) (map[string]string, error) {
	resMap := make(map[string]string)
	for k, v := range fields {
		if decoded, err := base64.StdEncoding.DecodeString(v); err != nil {
			return nil, err
		} else {
			resMap[k] = string(decoded)
		}
	}
	return resMap, nil
}

func labelsAnnotations(input api.ResourceInput, stack api.Stack) (map[string]string, map[string]string) {
	annotations := map[string]string{
		"pulumi.com/patchForce": "true",
		AnnotationEnv:           input.StackParams.Environment,
	}
	labels := map[string]string{
		LabelAppName: stack.Name,
		LabelAppType: AppTypeSimpleContainer,
		LabelScEnv:   input.StackParams.Environment,
	}
	return labels, annotations
}
