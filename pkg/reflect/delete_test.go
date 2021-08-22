package reflect

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestCascadeDelete(t *testing.T) {
	tests := []struct {
		d           string
		concurrency int
		toDelete    string
		namespaces  []string
		secrets     []*v1.Secret
		err         error
	}{}
	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			buf := bytes.NewBuffer([]byte{})
			l := zerolog.New(buf)

			objs := []runtime.Object{}
			for _, s := range test.secrets {
				objs = append(objs, s)
			}
			client := fake.NewSimpleClientset(objs...)

			assert.Equal(
				t,
				test.err,
				cascadeDelete(
					ctx,
					l,
					client.CoreV1(),
					test.toDelete,
					test.namespaces,
					test.concurrency))
		})
	}
}

func TestDeleteSecret(t *testing.T) {
	tests := []struct {
		d   string
		err error
	}{
		{
			"deletes a secret",
			nil,
		},
		{
			"fails to delete a secret",
			errors.New("deletion failure"),
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			s := "some-secret"
			secret := &v1.Secret{}
			secret.Name = s
			secret.Namespace = "default"
			buf := bytes.NewBuffer([]byte{})
			l := zerolog.New(buf)
			wg := sync.WaitGroup{}
			client := fake.NewSimpleClientset(secret)
			if test.err != nil {
				client.PrependReactor("delete", "*",
					func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.err
					})
			}
			errChan := make(chan error, 2)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deleteSecret(ctx, l, &wg, client.CoreV1(), s, "default", errChan)

			wg.Wait()
			select {
			case err := <-errChan:
				t.Log("received error")
				if test.err == nil {
					t.Logf("failed with err: %s", err.Error())
					t.Fail()
				}
			default:
				if test.err != nil {
					t.Log("failed to get error")
					t.Fail()
					return
				}
				t.Log("test succeded!")
			}
		})
	}
}
