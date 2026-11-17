.PHONY: bundle check-bundle test vet apk install

GOGIO_REF := 0magnet
APPID := com.skycoin.cxplayground
ARCH ?= arm64,amd64
APK ?= build/cx-playground.apk
ANDROID_HOME ?= /opt/android-sdk
ANDROID_NDK_ROOT ?= $(ANDROID_HOME)/ndk/30.0.16248370
export ANDROID_HOME ANDROID_NDK_ROOT

bundle: ## Rebuild the embedded app bundle from apps/
	go run ./cmd/cxmanifest bundle -o cmd/cxplayground/bundle apps/*

check-bundle: ## Fail if the committed bundle is stale
	go run ./cmd/cxmanifest bundle -o build/bundle-check apps/* > /dev/null
	diff -r build/bundle-check cmd/cxplayground/bundle

test: ## Run the tests, including every bundled CX app headless
	go test ./...

vet:
	go vet ./...

build/gogio/gogio:
	GOBIN=$(CURDIR)/build/gogio GOFLAGS= GOPROXY=direct go install github.com/0magnet/gio-cmd/gogio@$(GOGIO_REF)

apk: build/gogio/gogio check-bundle ## Build the APK for $(ARCH)
	GOFLAGS=-mod=vendor ./build/gogio/gogio -target android -arch $(ARCH) -minsdk 26 \
		-appid $(APPID) -name "CX Playground" -o $(APK) ./cmd/cxplayground

install: apk ## Install the APK on the adb device and start it
	adb install -r $(APK)
	adb shell am start -n $(APPID)/org.gioui.GioActivity
