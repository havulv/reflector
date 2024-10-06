package reflect

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/havulv/reflector/pkg/annotations"
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
				fmt.Printf("%s %s\n", test.toDelete, test.namespaces[len(test.namespaces)-1])
				sec, err := client.CoreV1().Secrets(test.namespaces[len(test.namespaces)-1]).Get(
					ctx, test.toDelete, metav1.GetOptions{})
				assert.True(t, apierrors.IsNotFound(err))
				assert.NotNil(t, sec)
				assert.Equal(t, sec.Name, "")
			}
		})
	}
}

func TestDeleteSecret(t *testing.T) {
	tests := []struct {
		d           string
		insertedErr error
		err         string
	}{
		{
			"deletes a secret",
			nil,
			"",
		},
		{
			"fails to delete a secret",
			&apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason:  metav1.StatusReasonNotFound,
					Message: "not found",
				},
			},
			"error while removing secret from the namspace: not found",
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
			client := fake.NewClientset(secret)
			if test.insertedErr != nil {
				client.PrependReactor("delete", "*",
					func(_ clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.insertedErr
					})
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := deleteSecret(ctx, client.CoreV1(), s, "default")

			if test.err != "" {
				require.NotNil(t, err)
				assert.EqualError(t, err, test.err)
				return
			}
			assert.Nil(t, err)
		})
	}
}

func TestFindExistingSecretNamespaces(t *testing.T) {
	tests := []struct {
		descrip       string
		name          string
		namespaces    []string
		listErr       error
		secretErr     error
		retSecrets    []*v1.Secret
		retNamespaces *v1.NamespaceList
	}{
		{
			"finds no namespaces to delete from",
			"thing",
			[]string{},
			nil,
			nil,
			[]*v1.Secret{},
			&v1.NamespaceList{
				Items: []v1.Namespace{},
			},
		},
		{
			"returns errors when trying to list all namespaces",
			"thing",
			[]string{},
			errors.New("some Error"),
			nil,
			[]*v1.Secret{},
			&v1.NamespaceList{
				Items: []v1.Namespace{},
			},
		},
		{
			"returns errors when trying to fetch a secret",
			"thing",
			[]string{},
			nil,
			errors.New("some Error"),
			[]*v1.Secret{},
			&v1.NamespaceList{
				Items: []v1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ns1",
						},
					},
				},
			},
		},
		{
			"returns a list of namespaces with secrets",
			"thing",
			[]string{"ns1"},
			nil,
			nil,
			[]*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thing",
						Namespace: "ns1",
						Annotations: map[string]string{
							annotations.ReflectionOwnerAnnotation: annotations.ReflectionOwned,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thing",
						Namespace: "ns2",
						Annotations: map[string]string{
							annotations.ReflectionOwnerAnnotation: "other",
						},
					},
				},
			},
			&v1.NamespaceList{
				Items: []v1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ns1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ns2",
						},
					},
				},
			},
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
			defer cancel()

			client := fake.NewSimpleClientset()
			if len(test.retSecrets) > 0 {
				tr := client.Tracker()
				for _, s := range test.retSecrets {
					assert.Nil(t, tr.Add(s))
				}
			}

			if test.retNamespaces != nil {
				tr := client.Tracker()
				assert.Nil(t, tr.Add(test.retNamespaces))
			}

			if test.listErr != nil {
				client.PrependReactor("list", "*",
					func(_ clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.listErr
					})
			}

			if test.secretErr != nil {
				client.PrependReactor("get", "*",
					func(_ clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.secretErr
					})
			}
			ns, err := findExistingSecretNamespaces(ctx, client.CoreV1(), test.name)
			if test.listErr != nil || test.secretErr != nil {
				assert.NotNil(t, err)
				return
			}
			assert.Equal(t, test.namespaces, ns)
		})
	}
}
