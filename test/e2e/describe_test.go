// Copyright 2022 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"testing"
	"time"

	regname "github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/carvel-imgpkg/test/helpers"
)

func TestDescribe(t *testing.T) {
	logger := &helpers.Logger{}

	t.Run("bundle with a single image", func(t *testing.T) {
		env := helpers.BuildEnv(t)
		imgpkg := helpers.Imgpkg{T: t, L: helpers.Logger{}, ImgpkgPath: env.ImgpkgPath}
		defer env.Cleanup()

		bundleTag := fmt.Sprintf(":%d", time.Now().UnixNano())
		var bundleDigest, imageDigest string
		logger.Section("create bundle with image", func() {
			imageDigest = env.ImageFactory.PushSimpleAppImageWithRandomFile(imgpkg, env.Image)

			imageLockYAML := fmt.Sprintf(`---
apiVersion: imgpkg.carvel.dev/v1alpha1
kind: ImagesLock
images:
- image: %s%s
  annotations:
    some.other.annotation: some other value
    some.annotation: some value
`, env.Image, imageDigest)
			bundleDir := env.BundleFactory.CreateBundleDir(helpers.BundleYAML, imageLockYAML)

			out := imgpkg.Run([]string{"push", "--tty", "-b", fmt.Sprintf("%s%s", env.Image, bundleTag), "-f", bundleDir})
			bundleDigest = fmt.Sprintf("@%s", helpers.ExtractDigest(t, out))
		})

		logger.Section("copy bundle to repository", func() {
			imgpkg.Run([]string{"copy",
				"--bundle", fmt.Sprintf("%s%s", env.Image, bundleDigest),
				"--to-repo", env.RelocationRepo},
			)
		})

		logger.Section("executes describe command", func() {
			stdout := imgpkg.Run(
				[]string{"describe",
					"--tty", "--bundle", fmt.Sprintf("%s%s", env.RelocationRepo, bundleDigest),
				},
			)
			require.Equal(t, fmt.Sprintf(`Bundle SHA: %s

Images:
  Image: %s%s
  Type: Image
  Origin: %s%s
  Annotations:
    some.annotation: some value
    some.other.annotation: some other value

Succeeded
`, bundleDigest[1:], env.RelocationRepo, imageDigest, env.Image, imageDigest), stdout)
		})
	})

	t.Run("bundle with bundle collocated", func(t *testing.T) {
		env := helpers.BuildEnv(t)
		imgpkg := helpers.Imgpkg{T: t, L: helpers.Logger{}, ImgpkgPath: env.ImgpkgPath}
		defer env.Cleanup()

		imgRef, err := regname.ParseReference(env.Image)
		require.NoError(t, err)

		var img1DigestRef, img2DigestRef, img1Digest, img2Digest string
		logger.Section("create 2 simple images", func() {
			img1DigestRef = imgRef.Context().Name() + "-img1"
			img1Digest = env.ImageFactory.PushSimpleAppImageWithRandomFile(imgpkg, img1DigestRef)
			img1DigestRef = img1DigestRef + img1Digest

			img2DigestRef = imgRef.Context().Name() + "-img2"
			img2Digest = env.ImageFactory.PushSimpleAppImageWithRandomFile(imgpkg, img2DigestRef)
			img2DigestRef = img2DigestRef + img2Digest
		})

		nestedBundle := imgRef.Context().Name() + "-bundle-nested"
		nestedBundleDigest := ""
		logger.Section("create nested bundle", func() {
			imageLockYAML := fmt.Sprintf(`---
apiVersion: imgpkg.carvel.dev/v1alpha1
kind: ImagesLock
images:
- image: %s
- image: %s
`, img1DigestRef, img2DigestRef)

			bundleDir := env.BundleFactory.CreateBundleDir(helpers.BundleYAML, imageLockYAML)
			out := imgpkg.Run([]string{"push", "--tty", "-b", nestedBundle, "-f", bundleDir})
			nestedBundleDigest = fmt.Sprintf("@%s", helpers.ExtractDigest(t, out))
		})

		outerBundle := imgRef.Context().Name() + "-bundle-outer"
		outerBundleDigest := ""
		logger.Section("create outer bundle", func() {
			imageLockYAML := fmt.Sprintf(`---
apiVersion: imgpkg.carvel.dev/v1alpha1
kind: ImagesLock
images:
- image: %s
  annotations:
    what is this: this is the nested bundle
- image: %s
  annotations:
    what is this: this is just an image
`, nestedBundle+nestedBundleDigest, img1DigestRef)

			bundleDir := env.BundleFactory.CreateBundleDir(helpers.BundleYAML, imageLockYAML)
			out := imgpkg.Run([]string{"push", "--tty", "-b", outerBundle, "-f", bundleDir})
			outerBundleDigest = fmt.Sprintf("@%s", helpers.ExtractDigest(t, out))
		})

		logger.Section("copy bundle to a different repository", func() {
			imgpkg.Run([]string{"copy", "-b", outerBundle + outerBundleDigest, "--to-repo", env.RelocationRepo})
		})

		logger.Section("executes describe command", func() {
			stdout := imgpkg.Run(
				[]string{"describe",
					"--tty", "--bundle", fmt.Sprintf("%s%s", env.RelocationRepo, outerBundleDigest),
				},
			)
			fmt.Println(stdout)
			require.Equal(t, fmt.Sprintf(`Bundle SHA: %s

Images:
  Image: %s%s
  Type: Bundle
  Origin: %s%s
  Annotations:
    what is this: this is the nested bundle
  Images:
    Image: %s%s
    Type: Image
    Origin: %s
    Image: %s%s
    Type: Image
    Origin: %s
  Image: %s%s
  Type: Image
  Origin: %s
  Annotations:
    what is this: this is just an image

Succeeded
`,
				outerBundleDigest[1:],
				env.RelocationRepo, nestedBundleDigest, nestedBundle, nestedBundleDigest,
				env.RelocationRepo, img1Digest, img1DigestRef,
				env.RelocationRepo, img2Digest, img2DigestRef,
				env.RelocationRepo, img1Digest, img1DigestRef,
			), stdout)
		})
	})

	t.Run("bundle with bundle NOT collocated", func(t *testing.T) {
		env := helpers.BuildEnv(t)
		imgpkg := helpers.Imgpkg{T: t, L: helpers.Logger{}, ImgpkgPath: env.ImgpkgPath}
		defer env.Cleanup()

		imgRef, err := regname.ParseReference(env.Image)
		require.NoError(t, err)

		var img1DigestRef, img2DigestRef, img1Digest, img2Digest string
		logger.Section("create 2 simple images", func() {
			img1DigestRef = imgRef.Context().Name() + "-img1"
			img1Digest = env.ImageFactory.PushSimpleAppImageWithRandomFile(imgpkg, img1DigestRef)
			img1DigestRef = img1DigestRef + img1Digest

			img2DigestRef = imgRef.Context().Name() + "-img2"
			img2Digest = env.ImageFactory.PushSimpleAppImageWithRandomFile(imgpkg, img2DigestRef)
			img2DigestRef = img2DigestRef + img2Digest
		})

		nestedBundle := imgRef.Context().Name() + "-bundle-nested"
		nestedBundleDigest := ""
		logger.Section("create nested bundle", func() {
			imageLockYAML := fmt.Sprintf(`---
apiVersion: imgpkg.carvel.dev/v1alpha1
kind: ImagesLock
images:
- image: %s
- image: %s
`, img2DigestRef, img1DigestRef)

			bundleDir := env.BundleFactory.CreateBundleDir(helpers.BundleYAML, imageLockYAML)
			out := imgpkg.Run([]string{"push", "--tty", "-b", nestedBundle, "-f", bundleDir})
			nestedBundleDigest = fmt.Sprintf("@%s", helpers.ExtractDigest(t, out))
		})

		outerBundle := imgRef.Context().Name() + "-bundle-outer"
		outerBundleDigest := ""
		logger.Section("create outer bundle", func() {
			imageLockYAML := fmt.Sprintf(`---
apiVersion: imgpkg.carvel.dev/v1alpha1
kind: ImagesLock
images:
- image: %s
- image: %s
`, nestedBundle+nestedBundleDigest, img1DigestRef)

			bundleDir := env.BundleFactory.CreateBundleDir(helpers.BundleYAML, imageLockYAML)
			out := imgpkg.Run([]string{"push", "--tty", "-b", outerBundle, "-f", bundleDir})
			outerBundleDigest = fmt.Sprintf("@%s", helpers.ExtractDigest(t, out))
		})

		logger.Section("executes describe command", func() {
			stdout := imgpkg.Run(
				[]string{"describe",
					"--tty", "--bundle", fmt.Sprintf("%s%s", outerBundle, outerBundleDigest),
				},
			)
			require.Equal(t, fmt.Sprintf(`Bundle SHA: %s

Images:
  Image: %s%s
  Type: Bundle
  Origin: %s%s
  Images:
    Image: %s
    Type: Image
    Origin: %s
    Image: %s
    Type: Image
    Origin: %s
  Image: %s
  Type: Image
  Origin: %s

Succeeded
`,
				outerBundleDigest[1:],
				nestedBundle, nestedBundleDigest, nestedBundle, nestedBundleDigest,
				img1DigestRef, img1DigestRef,
				img2DigestRef, img2DigestRef,
				img1DigestRef, img1DigestRef,
			), stdout)
		})
	})
}
