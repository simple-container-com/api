package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// TestHelmChartConfig_Getters covers the namespace/values accessors on the
// embedded HelmChartConfig, including the nil-pointer cases.
func TestHelmChartConfig_Getters(t *testing.T) {
	RegisterTestingT(t)

	t.Run("populated values are returned by accessors", func(t *testing.T) {
		RegisterTestingT(t)
		ns := "db"
		opNs := "db-operator"
		cfg := &HelmChartConfig{
			NamespaceName:         &ns,
			OperatorNamespaceName: &opNs,
			ValuesMap:             HelmValues{"key": "value", "nested": map[string]any{"a": 1}},
		}

		Expect(cfg.Namespace()).ToNot(BeNil())
		Expect(*cfg.Namespace()).To(Equal("db"))
		Expect(cfg.OperatorNamespace()).ToNot(BeNil())
		Expect(*cfg.OperatorNamespace()).To(Equal("db-operator"))
		Expect(cfg.Values()).To(HaveKey("key"))
		Expect(cfg.Values()["key"]).To(Equal("value"))
	})

	t.Run("zero-value accessors return nil", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &HelmChartConfig{}

		Expect(cfg.Namespace()).To(BeNil())
		Expect(cfg.OperatorNamespace()).To(BeNil())
		Expect(cfg.Values()).To(BeNil())
	})
}

// TestHelmOperatorChart_InterfaceConformance pins that HelmChartConfig (embedded
// by every operator type) satisfies the HelmOperatorChart interface and that
// the interface dispatches to the right values.
func TestHelmOperatorChart_InterfaceConformance(t *testing.T) {
	RegisterTestingT(t)

	opNs := "operators"
	var chart HelmOperatorChart = &HelmChartConfig{
		OperatorNamespaceName: &opNs,
		ValuesMap:             HelmValues{"replicaCount": 3},
	}

	Expect(chart.OperatorNamespace()).ToNot(BeNil())
	Expect(*chart.OperatorNamespace()).To(Equal("operators"))
	Expect(chart.Values()).To(HaveKey("replicaCount"))
}

func TestReadHelmPostgresOperatorConfig(t *testing.T) {
	RegisterTestingT(t)

	in := &api.Config{Config: map[string]any{
		"kubeconfig":        kubeconfigYAML,
		"namespace":         "pg",
		"operatorNamespace": "pg-operator",
		"volumeSize":        "10Gi",
		"numberOfInstances": 3,
		"version":           "15",
		"pg_hba":            []string{"host all all 0.0.0.0/0 md5"},
		"initSQL":           "CREATE EXTENSION pgcrypto;",
		"values":            map[string]any{"foo": "bar"},
	}}

	out, err := ReadHelmPostgresOperatorConfig(in)

	Expect(err).ToNot(HaveOccurred())
	pg, ok := out.Config.(*HelmPostgresOperator)
	Expect(ok).To(BeTrue())
	Expect(pg.KubernetesConfig).ToNot(BeNil())
	Expect(pg.KubernetesConfig.Kubeconfig).To(Equal(kubeconfigYAML))
	Expect(pg.Namespace()).ToNot(BeNil())
	Expect(*pg.Namespace()).To(Equal("pg"))
	Expect(pg.OperatorNamespace()).ToNot(BeNil())
	Expect(*pg.OperatorNamespace()).To(Equal("pg-operator"))
	Expect(pg.VolumeSize).ToNot(BeNil())
	Expect(*pg.VolumeSize).To(Equal("10Gi"))
	Expect(pg.NumberOfInstances).ToNot(BeNil())
	Expect(*pg.NumberOfInstances).To(Equal(3))
	Expect(pg.Version).ToNot(BeNil())
	Expect(*pg.Version).To(Equal("15"))
	Expect(pg.PgHbaEntries).To(ConsistOf("host all all 0.0.0.0/0 md5"))
	Expect(pg.InitSQL).ToNot(BeNil())
	Expect(*pg.InitSQL).To(Equal("CREATE EXTENSION pgcrypto;"))
	Expect(pg.Values()).To(HaveKey("foo"))
}

func TestReadHelmRedisOperatorConfig(t *testing.T) {
	RegisterTestingT(t)

	in := &api.Config{Config: map[string]any{
		"kubeconfig": kubeconfigYAML,
		"namespace":  "redis",
		"values":     map[string]any{"auth": map[string]any{"enabled": true}},
	}}

	out, err := ReadHelmRedisOperatorConfig(in)

	Expect(err).ToNot(HaveOccurred())
	r, ok := out.Config.(*HelmRedisOperator)
	Expect(ok).To(BeTrue())
	Expect(r.KubernetesConfig).ToNot(BeNil())
	Expect(*r.Namespace()).To(Equal("redis"))
	Expect(r.Values()).To(HaveKey("auth"))
}

func TestReadHelmRabbitmqOperatorConfig(t *testing.T) {
	RegisterTestingT(t)

	in := &api.Config{Config: map[string]any{
		"kubeconfig": kubeconfigYAML,
		"namespace":  "rabbit",
		"replicas":   5,
	}}

	out, err := ReadHelmRabbitmqOperatorConfig(in)

	Expect(err).ToNot(HaveOccurred())
	rmq, ok := out.Config.(*HelmRabbitmqOperator)
	Expect(ok).To(BeTrue())
	Expect(*rmq.Namespace()).To(Equal("rabbit"))
	Expect(rmq.Replicas).ToNot(BeNil())
	Expect(*rmq.Replicas).To(Equal(5))
}

func TestReadHelmMongodbOperatorConfig(t *testing.T) {
	RegisterTestingT(t)

	in := &api.Config{Config: map[string]any{
		"kubeconfig": kubeconfigYAML,
		"namespace":  "mongo",
		"version":    "6.0",
		"replicas":   2,
	}}

	out, err := ReadHelmMongodbOperatorConfig(in)

	Expect(err).ToNot(HaveOccurred())
	mdb, ok := out.Config.(*HelmMongodbOperator)
	Expect(ok).To(BeTrue())
	Expect(*mdb.Namespace()).To(Equal("mongo"))
	Expect(mdb.Version).ToNot(BeNil())
	Expect(*mdb.Version).To(Equal("6.0"))
	Expect(mdb.Replicas).ToNot(BeNil())
	Expect(*mdb.Replicas).To(Equal(2))
}
