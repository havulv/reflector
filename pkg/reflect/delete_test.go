package reflect

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func namespaceGen(end int) []string {
	ns := []string{}
	for i := 0; i < end; i++ {
		ns = append(ns, fmt.Sprintf("test-ns-%d", i))
	}
	return ns
}

func secretGen(name string, nss []string) []*v1.Secret {
	secrets := []*v1.Secret{}
	for _, ns := range nss {
		secrets = append(secrets, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		})
	}
	return secrets
}

func TestCascadeDelete(t *testing.T) {
	tests := []struct {
		d           string
		concurrency int
		toDelete    string
		namespaces  []string
		secrets     []*v1.Secret
		err         error
	}{
		{
			"runs with concurrency of one",
			1,
			"some-secret",
			[]string{"logging", "monitoring", "default"},
			[]*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-secret",
						Namespace: "logging",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-secret",
						Namespace: "monitoring",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-secret",
						Namespace: "default",
					},
				},
			},
			nil,
		},
		{
			"runs with concurrency of ten",
			10,
			"some-secret",
			namespaceGen(33),
			secretGen("some-secret", namespaceGen(33)),
			nil,
		},
		{
			"does nothing on no namespaces",
			20,
			"some-secret",
			[]string{},
			secretGen("some-secret", namespaceGen(1)),
			nil,
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
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
			if test.err == nil && len(test.namespaces) > 0 {
				sec, err := client.CoreV1().Secrets(test.namespaces[len(test.namespaces)-1]).Get(
					ctx, test.toDelete, metav1.GetOptions{})
				assert.Nil(t, sec)
				assert.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}

func TestDeleteSecret(t *testing.T) {
	tests := []struct {
		d         string
		err       error
		expectErr error
	}{
		{
			"deletes a secret",
			nil,
			nil,
		},
		{
			"fails to delete a secret",
			errors.New("deletion failure"),
			errors.New("deletion failure"),
		},
		{
			"fails to find a secret",
			nil,
			&apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonNotFound,
				},
			},
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			s := "some-secret"
			secret := &v1.Secret{}
			secret.Name = s
			secret.Namespace = "default"
			buf := bytes.NewBuffer([]byte{})
			l := zerolog.New(buf)
			wg := sync.WaitGroup{}
			client := fake.NewSimpleClientset(secret)
			if test.expectErr != nil {
				client.PrependReactor("delete", "*",
					func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.expectErr
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
