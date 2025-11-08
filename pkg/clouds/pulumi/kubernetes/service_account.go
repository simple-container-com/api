package kubernetes

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type SimpleServiceAccountArgs struct {
	Name      string
	Namespace string
	Resources []string
	APIGroups []string
	Verbs     []string
}

type SimpleServiceAccount struct {
	sdk.ResourceState

	Name sdk.StringOutput
}

func NewSimpleServiceAccount(ctx *sdk.Context, name string, args *SimpleServiceAccountArgs, opts ...sdk.ResourceOption) (*SimpleServiceAccount, error) {
	account := &SimpleServiceAccount{}
	// Sanitize name to comply with Kubernetes RFC 1123 requirements (no underscores)
	sanitizedName := SanitizeK8sName(name)
	err := ctx.RegisterComponentResource("pkg:k8s/extensions:simpleServiceAccount", sanitizedName, account, opts...)
	if err != nil {
		return nil, err
	}

	namespace := args.Namespace
	if namespace == "" {
		namespace = "default"
	}
	// Also sanitize namespace
	namespace = SanitizeK8sName(namespace)

	// Create ServiceAccount
	serviceAccount, err := corev1.NewServiceAccount(ctx, sanitizedName, &corev1.ServiceAccountArgs{
		AutomountServiceAccountToken: sdk.Bool(true),
		Metadata: &metav1.ObjectMetaArgs{
			Namespace: sdk.String(namespace),
			Name:      sdk.String(sanitizedName),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Define ClusterRole
	apiGroups := args.APIGroups
	if len(apiGroups) == 0 {
		apiGroups = []string{"*"}
	}

	verbs := args.Verbs
	if len(verbs) == 0 {
		verbs = []string{"get", "list"}
	}

	saClusterRole, err := rbacv1.NewClusterRole(ctx, sanitizedName, &rbacv1.ClusterRoleArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      sdk.String(sanitizedName),
			Namespace: serviceAccount.Metadata.Namespace().Elem(),
		},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				ApiGroups: sdk.ToStringArray(apiGroups),
				Resources: sdk.ToStringArray(args.Resources),
				Verbs:     sdk.ToStringArray(verbs),
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Bind the ServiceAccount to the ClusterRole via ClusterRoleBinding
	saRbacName, err := rbacv1.NewClusterRoleBinding(ctx, sanitizedName, &rbacv1.ClusterRoleBindingArgs{
		RoleRef: &rbacv1.RoleRefArgs{
			Kind:     sdk.String("ClusterRole"),
			Name:     saClusterRole.Metadata.Name().Elem(),
			ApiGroup: sdk.String("rbac.authorization.k8s.io"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Name:      serviceAccount.Metadata.Name().Elem(),
				Namespace: serviceAccount.Metadata.Namespace().Elem(),
				Kind:      sdk.String("ServiceAccount"),
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Register the outputs
	account.Name = serviceAccount.Metadata.Name().Elem()
	err = ctx.RegisterResourceOutputs(account, sdk.Map{
		"name":            serviceAccount.Metadata.Name(),
		"roleName":        saClusterRole.Metadata.Name(),
		"roleBindingName": saRbacName.Metadata.Name(),
	})
	if err != nil {
		return nil, err
	}

	return account, nil
}
