all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
    golang.mk \
    targets/openshift/images.mk \
    targets/openshift/deps.mk \
)

$(call build-image,$(GO_PACKAGE),./Dockerfile,.)

clean:
	$(RM) ./ci-monitor-operator
.PHONY: clean

GO_TEST_PACKAGES :=./pkg/... ./cmd/...
