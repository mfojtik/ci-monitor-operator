all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/library-go/alpha-build-machinery/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

$(call build-image,$(GO_PACKAGE),./Dockerfile,.)

clean:
	$(RM) ./config-history-operator
.PHONY: clean

GO_TEST_PACKAGES :=./pkg/... ./cmd/...
