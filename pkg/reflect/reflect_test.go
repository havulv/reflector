package reflect

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/havulv/reflector/pkg/annotations"
)

func TestReflectToNamespaces(t *testing.T) {
	tests := []struct {
		d         string
		earlyExit bool
	}{
		{
			"tests that no namespaces exits early",
			true,
		},
		{
			"tests that reflection is called",
			false,
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			client.PrependReactor("*", "*",
				func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, errors.New("some error")
				})

			namespaces := []string{"a", "b", "c", "d", "e"}
			if test.earlyExit {
				namespaces = []string{}
			}

			err := reflectToNamespaces(
				ctx, zerolog.New(bytes.NewBuffer([]byte{})),
				client.CoreV1(),
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "this",
						Namespace:   "thing",
						Annotations: map[string]string{},
					},
				}, namespaces, 2)

			if test.earlyExit {
				assert.Nil(t, err)
				return
			}
			assert.NotNil(t, err)

		})
	}
}

func TestReflectLambda(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := fake.NewSimpleClientset()
	client.PrependReactor("*", "*",
		func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("some error")
		})

	f := reflectLambda(
		ctx,
		zerolog.New(bytes.NewBuffer([]byte{})),
		client.CoreV1(),
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "this",
				Namespace:   "thing",
				Annotations: map[string]string{},
			},
		}, "some-hash")
	assert.NotNil(t, f)

	wg := &sync.WaitGroup{}
	errChan := make(chan error, 2)
	f(wg, "blergh", errChan)
	wg.Wait()

	select {
	case err := <-errChan:
		assert.NotNil(t, err)
	default:
		t.Log("no error received after goroutine completion")
		t.Fail()
	}

}

func TestReflectSecret(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := fake.NewSimpleClientset()
	client.PrependReactor("*", "*",
		func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("some error")
		})

	wg := &sync.WaitGroup{}
	errChan := make(chan error, 2)
	reflectSecret(
		ctx,
		zerolog.New(bytes.NewBuffer([]byte{})),
		wg,
		client.CoreV1(),
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "this",
				Namespace:   "thing",
				Annotations: map[string]string{},
			},
		},
		"hash",
		"blergh",
		errChan)
	wg.Wait()

	select {
	case err := <-errChan:
		assert.NotNil(t, err)
	default:
		t.Log("no error received after goroutine completion")
		t.Fail()
	}

}

func TestInstrumentedReflect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	name := "x"
	ns := "blergher"
	assert.Nil(t, instrumentedReflect(
		ctx,
		zerolog.New(bytes.NewBuffer([]byte{})),
		fake.NewSimpleClientset().CoreV1().Secrets("blergh"),
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   ns,
				Annotations: map[string]string{},
			},
		},
		"hash",
		"blergh"))
	m, err := reflectorSecretLatency.MetricVec.GetMetricWithLabelValues(name, ns)
	require.Nil(t, err)
	metric := &dto.Metric{}
	require.Nil(t, m.Write(metric))
	assert.NotNil(t, metric.Histogram)
}

func TestReflect(t *testing.T) {
	tests := []struct {
		d             string
		secret        *v1.Secret
		exists        bool
		hash          string
		getErr        error // error on fetching secret -- not found
		foundNoUpdate bool
	}{
		{
			"a reflection creates a new secret if the secret is not found",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "x",
					Namespace:   "blergh",
					Annotations: map[string]string{},
				},
			},
			false,
			"some-hash",
			nil,
			false,
		},
		{
			"a failure to get the secret results in no reflection",
			&v1.Secret{},
			false,
			"some-hash",
			errors.New("some get err"),
			false,
		},
		{
			"a found secret that doesn't need update is not reflected",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x",
					Namespace: "new-ns",
					Annotations: map[string]string{
						annotations.ReflectionHashAnnotation: "some-hash",
					},
				},
			},
			true,
			"some-hash",
			nil,
			true,
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewSimpleClientset()
			if test.exists {
				client = fake.NewSimpleClientset(test.secret)
			}

			if test.getErr != nil {
				client.PrependReactor("*", "*",
					func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.getErr
					})
			}
			buf := bytes.NewBuffer([]byte{})
			err := reflect(
				ctx,
				zerolog.New(buf),
				client.CoreV1().Secrets("new-ns"),
				test.secret,
				"some-hash",
				"new-ns")
			if test.getErr != nil {
				assert.NotNil(t, err)
				return
			}
			if test.foundNoUpdate {
				assert.Contains(t, buf.String(), "not updating")
			}
			assert.Nil(t, err)
			sec, err := client.CoreV1().Secrets("new-ns").Get(
				ctx, test.secret.Name, metav1.GetOptions{})
			assert.NotNil(t, sec)
		})
	}
}

func TestSecretNeedsUpdate(t *testing.T) {
	tests := []struct {
		d   string
		sec *v1.Secret
		res bool
	}{
		{
			"an unowned secret does not update",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"some-other-annotation": "thing",
					},
				},
			},
			false,
		},
		{
			"an unchanged hash does not update",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.ReflectionHashAnnotation:  "some-hash",
						annotations.ReflectionOwnerAnnotation: annotations.ReflectionOwned,
					},
				},
			},
			false,
		},
		{
			"changed hash does update",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.ReflectionHashAnnotation:  "other-hash",
						annotations.ReflectionOwnerAnnotation: annotations.ReflectionOwned,
					},
				},
			},
			true,
		},
		{
			"no explicit owner does not update",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.ReflectionHashAnnotation:  "other-hash",
						annotations.ReflectionOwnerAnnotation: "oofta",
					},
				},
			},
			false,
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			assert.Equal(
				t,
				test.res,
				secretNeedsUpdate(
					zerolog.New(bytes.NewBuffer([]byte{})),
					test.sec,
					"some-hash"))
		})
	}
}

func TestCreateNewSecret(t *testing.T) {
	tests := []struct {
		d  string
		og *v1.Secret
	}{
		{
			"creates a new secret",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "x",
					Namespace:   "blergh",
					Annotations: map[string]string{},
				},
			},
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			hash := "this"
			namespace := "blergh2"
			s := createNewSecret(test.og, hash, namespace)
			assert.Equal(t, s.Annotations[annotations.ReflectionHashAnnotation], hash)
			assert.Greater(t, len(s.Annotations[annotations.ReflectedAtAnnotation]), 0)
			assert.Equal(t, s.Annotations[annotations.ReflectedFromAnnotation], test.og.Namespace)
		})
	}
}

func TestCreateOrUpdateSecret(t *testing.T) {
	tests := []struct {
		d      string
		exists bool
		secret *v1.Secret
		err    error
	}{
		{
			"updates secret",
			true,
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "monitoring",
				},
				Data: map[string][]byte{
					"some-key": []byte("this is some data"),
				},
			},
			nil,
		},
		{
			"updates secret but hits error",
			true,
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "monitoring",
				},
				Data: map[string][]byte{
					"some-key": []byte("this is some data"),
				},
			},
			errors.New("this is an error"),
		},
		{
			"creates secret",
			false,
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "monitoring",
				},
				Data: map[string][]byte{
					"some-key": []byte("this is some data"),
				},
			},
			nil,
		},
		{
			"creates secret but hits error",
			false,
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "monitoring",
				},
				Data: map[string][]byte{
					"some-key": []byte("this is some data"),
				},
			},
			errors.New("this is an error"),
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client := fake.NewSimpleClientset()
			if test.exists {
				c := test.secret.DeepCopy()
				c.Data = map[string][]byte{
					"some_thing": []byte("other thing"),
				}
				client = fake.NewSimpleClientset(c)
			}
			if test.err != nil {
				client.PrependReactor("*", "*",
					func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.err
					})
			}

			err := createOrUpdateSecret(
				ctx,
				client.CoreV1().Secrets("monitoring"),
				test.secret,
				test.exists)
			if test.err != nil {
				assert.NotNil(t, err)
				return
			}
			assert.Nil(t, err)
			s, err := client.CoreV1().Secrets("monitoring").Get(ctx, test.secret.Name, metav1.GetOptions{})
			require.Nil(t, err)
			assert.Equal(t, s, test.secret)
		})
	}
}
