package reflect

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/havulv/reflector/pkg/annotations"
)

func cascadeDelete(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.SecretsGetter,
	secret string,
	namespaces []string,
	concurrency int,
) error {
	// shortcircuit if we have the best case of `do nothing`
	if len(namespaces) == 0 {
		logger.Info().Msg("no namespaces, skipping")
		return nil
	}

	return batchOverNamespaces(
		ctx,
		logger,
		concurrency,
		namespaces,
		deleteLambda(client, secret))
}

func deleteLambda(
	client corev1.SecretsGetter,
	secret string,
) func(context.Context, string) error {
	return func(ctx context.Context, ns string) error {
		// spin off a goroutine for every level of concurrency
		return deleteSecret(
			ctx, client, secret, ns)
	}
}

func deleteSecret(
	ctx context.Context,
	client corev1.SecretsGetter,
	secret string,
	ns string,
) error {
	// Not found errors are returned as errors, so that the
	// caller can deal with them upstream. In the batch form
	// it will just be logged, so it doesn't really matter there.
	secretClient := client.Secrets(ns)
	if err := secretClient.Delete(
		ctx, secret, metav1.DeleteOptions{},
	); err != nil {
		return fmt.Errorf(
			"error while removing secret from the namspace: %w", err)
	}
	return nil
}

func findExistingSecretNamespaces(
	ctx context.Context,
	core corev1.CoreV1Interface,
	name string,
) ([]string, error) {
	// fetch all namespaces
	allNs, err := core.Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return []string{}, fmt.Errorf("unable to list all namespaces: %w", err)
	}

	namespaces := []string{}
	for _, item := range allNs.Items {
		found, err := core.Secrets(item.Name).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return []string{}, fmt.Errorf("could not fetch secret for ns: %w", err)
		}
		if annotations.CanOperate(found.Annotations) {
			namespaces = append(namespaces, item.Name)
		}
	}

	return namespaces, nil
}
