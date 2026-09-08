package kvstore_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justtrackio/gosoline/pkg/kvstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	configMapTestNamespace = "test-ns"
	configMapTestStore     = "test"
	// kvstore-test- + 50 chars = 63 chars, the kubernetes spec name length limit
	configMapMaxTestKeyLength = kvstore.ConfigMapMaxKeyLength - len("kvstore-test-")
)

func buildTestableConfigMapStore[T any](t *testing.T, data map[string]string) (context.Context, kvstore.KvStore[T], *fake.Clientset) {
	t.Helper()

	client := fake.NewSimpleClientset()

	if data != nil {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("kvstore-%s", configMapTestStore),
				Namespace: configMapTestNamespace,
			},
			Data: data,
		}
		_, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Create(t.Context(), cm, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	store, err := kvstore.NewConfigMapKvStoreWithClient[T](client, configMapTestNamespace, configMapTestStore, &kvstore.ConfigMapSettings{
		BatchSize: 100,
	})
	require.NoError(t, err)

	return t.Context(), store, client
}

func TestConfigMapKvStore_PutAndGet(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, nil)

	err := store.Put(ctx, "foo", Item{Id: "foo", Body: "bar"})
	require.NoError(t, err)

	// the value is stored under the prefixed data key
	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, cm.Data, "kvstore-test-foo")
	assert.Equal(t, `{"id":"foo","body":"bar"}`, cm.Data["kvstore-test-foo"])

	item := &Item{}
	found, err := store.Get(ctx, "foo", item)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "foo", item.Id)
	assert.Equal(t, "bar", item.Body)
}

func TestConfigMapKvStore_GetMissing(t *testing.T) {
	// configmap exists but is empty
	ctx, store, _ := buildTestableConfigMapStore[Item](t, map[string]string{})

	item := &Item{}
	found, err := store.Get(ctx, "missing", item)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, Item{}, *item)
}

func TestConfigMapKvStore_ReadMissingConfigMap(t *testing.T) {
	// no configmap exists yet -> reads return a clear error
	ctx, store, _ := buildTestableConfigMapStore[Item](t, nil)

	item := &Item{}
	_, err := store.Get(ctx, "foo", item)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	exists, err := store.Contains(ctx, "foo")
	require.Error(t, err)
	assert.False(t, exists)
	assert.Contains(t, err.Error(), "not found")
}

func TestConfigMapKvStore_GetUnmarshalsStoredData(t *testing.T) {
	ctx, store, _ := buildTestableConfigMapStore[Item](t, map[string]string{
		"kvstore-test-foo": `{"id":"foo","body":"bar"}`,
	})

	item := &Item{}
	found, err := store.Get(ctx, "foo", item)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "bar", item.Body)
}

func TestConfigMapKvStore_Contains(t *testing.T) {
	ctx, store, _ := buildTestableConfigMapStore[Item](t, map[string]string{
		"kvstore-test-bar": `{"id":"bar","body":"baz"}`,
	})

	exists, err := store.Contains(ctx, "bar")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = store.Contains(ctx, "foo")
	require.NoError(t, err)
	assert.False(t, exists)

	// foreign data keys in the same configmap are ignored
	exists, err = store.Contains(ctx, "store2-foo")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestConfigMapKvStore_Delete(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, map[string]string{
		"kvstore-test-foo": `{"id":"foo","body":"bar"}`,
	})

	err := store.Delete(ctx, "foo")
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, cm.Data, "kvstore-test-foo")

	found, err := store.Contains(ctx, "foo")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestConfigMapKvStore_DeleteMissingConfigMap(t *testing.T) {
	ctx, store, _ := buildTestableConfigMapStore[Item](t, nil)

	// deleting from a store whose configmap does not exist yet is a no-op
	err := store.Delete(ctx, "foo")
	require.NoError(t, err)
}

func TestConfigMapKvStore_PutBatch(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, nil)

	err := store.PutBatch(ctx, map[string]Item{
		"foo": {Id: "foo", Body: "bar"},
		"fuu": {Id: "fuu", Body: "baz"},
	})
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"kvstore-test-foo": `{"id":"foo","body":"bar"}`,
		"kvstore-test-fuu": `{"id":"fuu","body":"baz"}`,
	}, cm.Data)

	result := map[string]Item{}
	missing, err := store.GetBatch(ctx, []string{"foo", "fuu"}, result)
	require.NoError(t, err)
	assert.Empty(t, missing)
	assert.Equal(t, "bar", result["foo"].Body)
	assert.Equal(t, "baz", result["fuu"].Body)
}

func TestConfigMapKvStore_GetBatch(t *testing.T) {
	ctx, store, _ := buildTestableConfigMapStore[Item](t, map[string]string{
		"kvstore-test-foo": `{"id":"foo","body":"bar"}`,
	})

	result := map[string]Item{}
	missing, err := store.GetBatch(ctx, []string{"foo", "fuu"}, result)
	require.NoError(t, err)
	require.Len(t, missing, 1)
	assert.Contains(t, missing, "fuu")
	assert.Equal(t, "bar", result["foo"].Body)
	assert.NotContains(t, result, "fuu")
}

func TestConfigMapKvStore_GetBatchInvalidKey(t *testing.T) {
	// configmap exists, but the key exceeds the length limit
	ctx, store, _ := buildTestableConfigMapStore[Item](t, map[string]string{})

	longKey := strings.Repeat("a", configMapMaxTestKeyLength+1)

	_, err := store.GetBatch(ctx, []string{longKey}, map[string]Item{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum key name length")
}

func TestConfigMapKvStore_DeleteBatch(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, map[string]string{
		"kvstore-test-foo": `{"id":"foo","body":"bar"}`,
		"kvstore-test-fuu": `{"id":"fuu","body":"baz"}`,
	})

	err := store.DeleteBatch(ctx, []string{"foo", "fuu", "missing"})
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, cm.Data)
}

func TestConfigMapKvStore_EstimateSize(t *testing.T) {
	_, store, _ := buildTestableConfigMapStore[Item](t, map[string]string{
		"kvstore-test-foo":  `{"id":"foo","body":"bar"}`,
		"kvstore-test-fuu":  `{"id":"fuu","body":"baz"}`,
		"kvstore-other-bar": `{"id":"bar","body":"baz"}`,
		"unrelated":         "value",
	})

	sizedStore, ok := store.(kvstore.SizedStore[Item])
	require.True(t, ok)

	assert.Equal(t, int64(2), *sizedStore.EstimateSize())
}

func TestConfigMapKvStore_KeyPrefix(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, nil)

	err := store.Put(ctx, "some-key", Item{Id: "some-key", Body: "body"})
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, cm.Data, 1)

	// the data key follows the pattern kvstore-<storeName>-<keyName>
	assert.Contains(t, cm.Data, "kvstore-test-some-key")
}

func TestConfigMapKvStore_KeyLengthValidation(t *testing.T) {
	ctx, store, _ := buildTestableConfigMapStore[Item](t, nil)

	// the maximum length fits exactly
	maxKey := strings.Repeat("a", configMapMaxTestKeyLength)
	err := store.Put(ctx, maxKey, Item{Id: maxKey, Body: "bar"})
	require.NoError(t, err)

	found, err := store.Contains(ctx, maxKey)
	require.NoError(t, err)
	assert.True(t, found)

	// one character too long is rejected
	tooLong := strings.Repeat("a", configMapMaxTestKeyLength+1)

	_, err = store.Get(ctx, tooLong, &Item{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum key name length")

	err = store.Put(ctx, tooLong, Item{Id: tooLong, Body: "bar"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum key name length")

	err = store.Delete(ctx, tooLong)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum key name length")

	exists, err := store.Contains(ctx, tooLong)
	require.Error(t, err)
	assert.False(t, exists)
	assert.Contains(t, err.Error(), "exceeds the maximum key name length")
}

func TestConfigMapKvStore_EmptyKey(t *testing.T) {
	ctx, store, _ := buildTestableConfigMapStore[Item](t, nil)

	err := store.Put(ctx, "", Item{Id: "", Body: "bar"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")

	exists, err := store.Contains(ctx, "")
	require.Error(t, err)
	assert.False(t, exists)
}

func TestConfigMapKvStore_ConstructorValidation(t *testing.T) {
	client := fake.NewSimpleClientset()

	_, err := kvstore.NewConfigMapKvStoreWithClient[Item](nil, configMapTestNamespace, configMapTestStore, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client is required")

	_, err = kvstore.NewConfigMapKvStoreWithClient[Item](client, "", configMapTestStore, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace must not be empty")

	_, err = kvstore.NewConfigMapKvStoreWithClient[Item](client, strings.Repeat("n", 64), configMapTestStore, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the kubernetes spec name length limit")

	// the configmap name is kvstore-<storeName>, so the store name must fit in
	// 63 - 7 - 1 = 55 chars
	_, err = kvstore.NewConfigMapKvStoreWithClient[Item](client, configMapTestNamespace, strings.Repeat("s", 56), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the maximum store name length")

	// nil settings are accepted and defaulted
	store, err := kvstore.NewConfigMapKvStoreWithClient[Item](client, configMapTestNamespace, configMapTestStore, nil)
	require.NoError(t, err)
	require.NotNil(t, store)
}

func TestConfigMapKvStore_MissingConfigMapIsCreated(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, nil)

	// no configmap exists yet
	_, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.Error(t, err)

	err = store.Put(ctx, "foo", Item{Id: "foo", Body: "bar"})
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, configMapTestNamespace, cm.Namespace)
	assert.Equal(t, map[string]string{
		"kvstore-test-foo": `{"id":"foo","body":"bar"}`,
	}, cm.Data)
}

func TestConfigMapKvStore_DataSizeLimit(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, nil)

	// a single value that pushes the data map beyond the 1MiB limit is rejected
	hugeBody := strings.Repeat("x", kvstore.ConfigMapMaxDataSize)
	err := store.Put(ctx, "huge", Item{Id: "huge", Body: hugeBody})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the kubernetes configmap size limit")

	// the configmap was not created
	_, err = client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.Error(t, err)
}

func TestConfigMapKvStore_PreservesForeignData(t *testing.T) {
	ctx, store, client := buildTestableConfigMapStore[Item](t, map[string]string{
		"some-other-entry": "keep-me",
	})

	err := store.Put(ctx, "foo", Item{Id: "foo", Body: "bar"})
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps(configMapTestNamespace).Get(ctx, "kvstore-test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "keep-me", cm.Data["some-other-entry"])
	assert.Equal(t, `{"id":"foo","body":"bar"}`, cm.Data["kvstore-test-foo"])
}

func TestConfigMapKvStore_UpdateError(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kvstore-test",
			Namespace: configMapTestNamespace,
		},
		Data: map[string]string{"kvstore-test-foo": `{"id":"foo","body":"bar"}`},
	})

	// fail all updates to simulate concurrent modification / api errors
	client.PrependReactor("update", "configmaps", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("api error")
	})

	store, err := kvstore.NewConfigMapKvStoreWithClient[Item](client, configMapTestNamespace, configMapTestStore, &kvstore.ConfigMapSettings{
		BatchSize: 100,
	})
	require.NoError(t, err)

	err = store.Put(t.Context(), "foo", Item{Id: "foo", Body: "baz"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can not update configmap")
}
