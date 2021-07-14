SNAP_ESBUILD_VERSION = $(shell cat version.snap.txt)

########
# NOTE #
########

# We don't build nor publish the following:
# - snap-platform-wasm
# - snap-platform-neutral
# - snap-platform-darwin-arm64 (M1)

snapshot: cmd/esbuild/version.go cmd/esbuild/*.go cmd/snapshot/*.go pkg/*/*.go internal/*/*.go go.mod 
	go build "-ldflags=-s -w" ./cmd/snapshot

snap-platform-all: cmd/esbuild/version.go
	make -j8 \
		snap-platform-windows \
		snap-platform-windows-32 \
		snap-platform-android-arm64 \
		snap-platform-darwin \
		snap-platform-freebsd \
		snap-platform-freebsd-arm64 \
		snap-platform-linux \
		snap-platform-linux-32 \
		snap-platform-linux-arm \
		snap-platform-linux-arm64 \
		snap-platform-linux-mips64le \
		snap-platform-linux-ppc64le

snap-platform-windows:
	cd npm/snapbuild-windows-64 && npm version "$(SNAP_ESBUILD_VERSION)" --allow-same-version
	GOOS=windows GOARCH=amd64 go build "-ldflags=-s -w" -o npm/snapbuild-windows-64/bin/snapshot.exe ./cmd/snapshot

snap-platform-windows-32:
	cd npm/snapbuild-windows-32 && npm version "$(SNAP_ESBUILD_VERSION)" --allow-same-version
	GOOS=windows GOARCH=386 go build "-ldflags=-s -w" -o npm/snapbuild-windows-32/bin/snapshot.exe ./cmd/snapshot

snap-platform-unixlike:
	test -n "$(GOOS)" && test -n "$(GOARCH)" && test -n "$(NPMDIR)"
	mkdir -p "$(NPMDIR)/bin"
	cd "$(NPMDIR)" && npm version "$(SNAP_ESBUILD_VERSION)" --allow-same-version
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build "-ldflags=-s -w" -o "$(NPMDIR)/bin/snapshot" ./cmd/snapshot

snap-platform-android-arm64:
	make GOOS=android GOARCH=arm64 NPMDIR=npm/snapbuild-android-arm64 snap-platform-unixlike

snap-platform-darwin:
	make GOOS=darwin GOARCH=amd64 NPMDIR=npm/snapbuild-darwin-64 snap-platform-unixlike

snap-platform-darwin-arm64:
	make GOOS=darwin GOARCH=arm64 NPMDIR=npm/snapbuild-darwin-arm64 snap-platform-unixlike

snap-platform-freebsd:
	make GOOS=freebsd GOARCH=amd64 NPMDIR=npm/snapbuild-freebsd-64 snap-platform-unixlike

snap-platform-freebsd-arm64:
	make GOOS=freebsd GOARCH=arm64 NPMDIR=npm/snapbuild-freebsd-arm64 snap-platform-unixlike

snap-platform-linux:
	make GOOS=linux GOARCH=amd64 NPMDIR=npm/snapbuild-linux-64 snap-platform-unixlike

snap-platform-linux-32:
	make GOOS=linux GOARCH=386 NPMDIR=npm/snapbuild-linux-32 snap-platform-unixlike

snap-platform-linux-arm:
	make GOOS=linux GOARCH=arm NPMDIR=npm/snapbuild-linux-arm snap-platform-unixlike

snap-platform-linux-arm64:
	make GOOS=linux GOARCH=arm64 NPMDIR=npm/snapbuild-linux-arm64 snap-platform-unixlike

snap-platform-linux-mips64le:
	make GOOS=linux GOARCH=mips64le NPMDIR=npm/snapbuild-linux-mips64le snap-platform-unixlike

snap-platform-linux-ppc64le:
	make GOOS=linux GOARCH=ppc64le NPMDIR=npm/snapbuild-linux-ppc64le snap-platform-unixlike

snap-publish-all: cmd/esbuild/version.go snap-test-prepublish
	@test thlorenz/snap = "`git rev-parse --abbrev-ref HEAD`" || (echo "Refusing to publish from non-snapshot branch `git rev-parse --abbrev-ref HEAD`" && false)
	@echo "Checking for unpushed commits..." && git fetch
	@test "" = "`git cherry`" || (echo "Refusing to publish with unpushed commits" && false)
	rm -fr npm && git checkout npm
	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" make -j4 \
		snap-publish-windows \
		snap-publish-windows-32 \
		snap-publish-freebsd \
		snap-publish-freebsd-arm64 \
		snap-publish-darwin
	@echo Enter one-time password:
	@read OTP && OTP="$$OTP" make -j4 \
		snap-publish-android-arm64 \
		snap-publish-linux \
		snap-publish-linux-32 \
		snap-publish-linux-arm \
		snap-publish-linux-arm64 \
		snap-publish-linux-mips64le \
		snap-publish-linux-ppc64le
	# We don't publish this module to npm and manage this via a separate package instead
	# git commit -am "publish $(SNAP_ESBUILD_VERSION) to npm"
	# git tag "v$(SNAP_ESBUILD_VERSION)"
	# git push origin master "v$(SNAP_ESBUILD_VERSION)"

snap-publish-windows: snap-platform-windows
	test -n "$(OTP)" && cd npm/snapbuild-windows-64 && npm publish --otp="$(OTP)"

snap-publish-windows-32: snap-platform-windows-32
	test -n "$(OTP)" && cd npm/snapbuild-windows-32 && npm publish --otp="$(OTP)"

snap-publish-android-arm64: snap-platform-android-arm64
	test -n "$(OTP)" && cd npm/snapbuild-android-arm64 && npm publish --otp="$(OTP)"

snap-publish-darwin: snap-platform-darwin
	test -n "$(OTP)" && cd npm/snapbuild-darwin-64 && npm publish --otp="$(OTP)"

snap-publish-darwin-arm64: snap-platform-darwin-arm64
	test -n "$(OTP)" && cd npm/snapbuild-darwin-arm64 && npm publish --otp="$(OTP)"

snap-publish-freebsd: snap-platform-freebsd
	test -n "$(OTP)" && cd npm/snapbuild-freebsd-64 && npm publish --otp="$(OTP)"

snap-publish-freebsd-arm64: snap-platform-freebsd-arm64
	test -n "$(OTP)" && cd npm/snapbuild-freebsd-arm64 && npm publish --otp="$(OTP)"

snap-publish-linux: snap-platform-linux
	test -n "$(OTP)" && cd npm/snapbuild-linux-64 && npm publish --otp="$(OTP)"

snap-publish-linux-32: snap-platform-linux-32
	test -n "$(OTP)" && cd npm/snapbuild-linux-32 && npm publish --otp="$(OTP)"

snap-publish-linux-arm: snap-platform-linux-arm
	test -n "$(OTP)" && cd npm/snapbuild-linux-arm && npm publish --otp="$(OTP)"

snap-publish-linux-arm64: snap-platform-linux-arm64
	test -n "$(OTP)" && cd npm/snapbuild-linux-arm64 && npm publish --otp="$(OTP)"

snap-publish-linux-mips64le: snap-platform-linux-mips64le
	test -n "$(OTP)" && cd npm/snapbuild-linux-mips64le && npm publish --otp="$(OTP)"

snap-publish-linux-ppc64le: snap-platform-linux-ppc64le
	test -n "$(OTP)" && cd npm/snapbuild-linux-ppc64le && npm publish --otp="$(OTP)"

snap-test-prepublish: check-go-version
