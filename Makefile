
GIT_TAG := $(shell git describe --abbrev=0)
TAG_DISTANCE := $(shell git describe --long | awk -F- '{print $$2}')
SRC_DIR := pkg
GENERIC_SRC_DIR := ${SRC_DIR}/generic
DEB_SRC_DIR := ${SRC_DIR}/debian
DEB_REPO_DIR := ${DEB_SRC_DIR}/repo
BUILD_DIR := build
BUILD_REPOS_DIR := build/repos
DEB_BUILD_DIR := ${BUILD_DIR}/debian
EXE := rackspace-monitoring-poller
APP_NAME := rackspace-monitoring-poller

CLOUDFILES_REPO_NAME := poller-$(GIT_TAG)
RCLONE_ARGS :=

PROJECT_VENDOR := github.com/racker/rackspace-monitoring-poller/vendor

PKGDIR_BIN := usr/bin
PKGDIR_ETC := etc

OS := linux
ARCH := amd64
BIN_URL := https://github.com/racker/rackspace-monitoring-poller/releases/download/$(GIT_TAG)/$(EXE)_$(OS)_$(ARCH)
VENDOR := Rackspace US, Inc.
LICENSE := Apache v2

# TODO: should poller get its own specific file?
APP_CFG := ${PKGDIR_ETC}/rackspace-monitoring-poller.cfg
SYSTEMD_CONF := lib/systemd/system/${APP_NAME}.service
UPSTART_CONF := ${PKGDIR_ETC}/init/${APP_NAME}
UPSTART_DEFAULT := ${PKGDIR_ETC}/default/${APP_NAME}
LOGROTATE_CFG := ${PKGDIR_ETC}/logrotate.d/${APP_NAME}

OWNED_DIRS :=
DEB_CONFIG_FILES := ${APP_CFG} ${LOGROTATE_CFG}

PKG_VERSION := ${GIT_TAG}-${TAG_DISTANCE}
PKG_BASE := ${APP_NAME}_${PKG_VERSION}_${ARCH}


define YAML_HDR
name: ${APP_NAME}
arch: ${ARCH}
platform: ${OS}
version: ${PKG_VERSION}
section: default 
priority: extra 
vendor: ${VENDOR}
license: ${LICENSE}
bindir: /${PKGDIR_BIN}
endef

define SYSTEMD_YAML_FILES
  ${DEB_BUILD_DIR}/${SYSTEMD_CONF}: /${SYSTEMD_CONF} 
endef

define YAML_FILES
files:
  ${DEB_BUILD_DIR}/${PKGDIR_BIN}/${EXE}: /${PKGDIR_BIN}/${EXE}
endef

define YAML_CONFIG_FILES
config_files:
  ${DEB_BUILD_DIR}/${UPSTART_DEFAULT}: /${UPSTART_DEFAULT}
  ${DEB_BUILD_DIR}/${APP_CFG}: /${APP_CFG}
  ${DEB_BUILD_DIR}/${LOGROTATE_CFG}: /${LOGROTATE_CFG}
endef

define UPSTART_YAML_CONFIG_FILES
  ${DEB_BUILD_DIR}/${UPSTART_CONF}: /${UPSTART_CONF}.conf
endef

define YAML_SCRIPTS
scripts:
        postinstall: ${DEB_SRC_DIR}/postinst
        preremove: ${DEB_SRC_DIR}/prerm
endef

define DEB_YAML
${YAML_HDR}
${YAML_FILES}
${SYSTEMD_YAML_FILES}
${YAML_CONFIG_FILES}
${UPSTART_YAML_CONFIG_FILES}
${YAML_SCRIPTS}
endef

define SYSTEMD_DEB_YAML
${YAML_HDR}
${YAML_FILES}
${SYSTEMD_YAML_FILES}
${YAML_CONFIG_FILES}
${YAML_SCRIPTS}
endef

define UPSTART_DEB_YAML
${YAML_HDR}
${YAML_FILES}
${YAML_CONFIG_FILES}
${UPSTART_YAML_CONFIG_FILES}
endef

export DEB_YAML
export SYSTEMD_DEB_YAML
export UPSTART_DEB_YAML

ifdef DONT_SIGN
  SED_DISTRIBUTIONS = -e "/SignWith/ d" -e "p"
else
  SED_DISTRIBUTIONS = -e "p"
endif

MOCK_POLLER := LogPrefixGetter,ConnectionStream,Connection,Session,CheckScheduler,CheckExecutor,Scheduler,ChecksReconciler

WGET := wget
NFPM := ${BUILD_DIR}/nfpm
REPREPRO := reprepro

default: clean package

generate-mocks: ${GOPATH}/bin/mockgen
	${GOPATH}/bin/mockgen -package=poller_test -destination=poller/poller_mock_test.go github.com/racker/rackspace-monitoring-poller/poller ${MOCK_POLLER}
	${GOPATH}/bin/mockgen -source=utils/events.go -package=utils -destination=utils/events_mock_test.go
	${GOPATH}/bin/mockgen -destination check/pinger_mock_test.go -package=check_test github.com/racker/rackspace-monitoring-poller/check Pinger
	sed -i '' s,$(PROJECT_VENDOR)/,, check/pinger_mock_test.go
	${GOPATH}/bin/mockgen -destination mock_golang/mock_conn.go -package mock_golang net Conn

test: vendor
	go test -short -v $(shell glide novendor)

test-integrationcli: build
	go test -v github.com/racker/rackspace-monitoring-poller/integrationcli

build: ${GOPATH}/bin/gox vendor
	CGO_ENABLED=0 ${GOPATH}/bin/gox \
	  -osarch "linux/386 linux/amd64 darwin/amd64 windows/386 windows/amd64" \
	  -output="${BUILD_DIR}/{{.Dir}}_{{.OS}}_{{.Arch}}"

coverage: ${GOPATH}/bin/goveralls
	contrib/combine-coverage.sh --coveralls

vendor: ${GOPATH}/bin/glide glide.yaml glide.lock
	${GOPATH}/bin/glide install

install-nfpm:
	wget -O ${BUILD_DIR}/nfpm.tar.gz https://github.com/goreleaser/nfpm/releases/download/v0.8.2/nfpm_0.8.2_Linux_x86_64.tar.gz
	tar -C ${BUILD_DIR} -xzf ${BUILD_DIR}/nfpm.tar.gz

${NFPM}: install-nfpm

${GOPATH}/bin/glide :
	curl https://glide.sh/get | sh

${GOPATH}/bin/gox :
	go get -v github.com/mitchellh/gox

${GOPATH}/bin/goveralls :
	go get -v github.com/mattn/goveralls

${GOPATH}/bin/mockgen :
	go get -v github.com/golang/mock/mockgen

regenerate-callgraphs : clean-callgraphs generate-callgraphs

generate-callgraphs : docs/poller_callgraph.png docs/endpoint_callgraph.png

clean-callgraphs :
	rm -f docs/*_callgraph.{dot,png}

%_callgraph.png : %_callgraph.dot
	dot -Tpng -o $@ $<

%_callgraph.dot : ${GOPATH}/bin/go-callvis
	${GOPATH}/bin/go-callvis -focus $(*F) -nostd -group type github.com/racker/rackspace-monitoring-poller > $@

${GOPATH}/bin/go-callvis :
	go get -u github.com/TrueFurby/go-callvis

package: package-debs

package-repo-upload: package reprepro-debs package-upload-deb

package-upload-deb:
	rclone ${RCLONE_ARGS} mkdir rackspace:${CLOUDFILES_REPO_NAME}
	rclone ${RCLONE_ARGS} copy ${BUILD_REPOS_DIR} rackspace:${CLOUDFILES_REPO_NAME}

reprepro-debs: \
	${BUILD_REPOS_DIR}/ubuntu-14.04-x86_64 \
	${BUILD_REPOS_DIR}/ubuntu-16.04-x86_64 \
	${BUILD_REPOS_DIR}/debian

clean-repos:
	rm -rf ${BUILD_REPOS_DIR}

# NOTE make 4.1 supports the proper syntax, which is
# define buildReprepro =
define buildReprepro
	mkdir -p $@/conf
	sed -n ${SED_DISTRIBUTIONS} pkg/debian/repo/conf/distributions > $@/conf/distributions
	${REPREPRO} -b $@ includedeb cloudmonitoring $<
endef

${BUILD_REPOS_DIR}/ubuntu-14.04-x86_64 : ${BUILD_DIR}/${PKG_BASE}_upstart.deb
	$(buildReprepro)

${BUILD_REPOS_DIR}/ubuntu-16.04-x86_64 : ${BUILD_DIR}/${PKG_BASE}_systemd.deb
	$(buildReprepro)

${BUILD_REPOS_DIR}/debian : ${BUILD_DIR}/${PKG_BASE}.deb
	$(buildReprepro)

package-debs: ${BUILD_DIR}/${PKG_BASE}.deb \
	${BUILD_DIR}/${PKG_BASE}_systemd.deb \
	${BUILD_DIR}/${PKG_BASE}_upstart.deb

package-debs-local: stage-deb-exe-local package-debs

${BUILD_DIR}/${PKG_BASE}.deb : $(addprefix ${DEB_BUILD_DIR}/,${PKGDIR_BIN}/${EXE} ${DEB_CONFIG_FILES} ${UPSTART_DEFAULT} ${UPSTART_CONF} ${SYSTEMD_CONF}) ${NFPM}
	rm -f $@
	echo "$$DEB_YAML" > ${BUILD_DIR}/deb.yaml
	${NFPM} -f ${BUILD_DIR}/deb.yaml pkg -t $@

${BUILD_DIR}/${PKG_BASE}_systemd.deb : $(addprefix ${DEB_BUILD_DIR}/,${PKGDIR_BIN}/${EXE} ${DEB_CONFIG_FILES} ${SYSTEMD_CONF}) ${NFPM}
	rm -f $@
	echo "$$SYSTEMD_DEB_YAML" > ${BUILD_DIR}/systemd_deb.yaml
	${NFPM} -f ${BUILD_DIR}/systemd_deb.yaml pkg -t $@

${BUILD_DIR}/${PKG_BASE}_upstart.deb : $(addprefix ${DEB_BUILD_DIR}/,${PKGDIR_BIN}/${EXE} ${DEB_CONFIG_FILES} ${UPSTART_DEFAULT} ${UPSTART_CONF}) ${NFPM}
	rm -f $@
	echo "$$UPSTART_DEB_YAML" > ${BUILD_DIR}/upstart_deb.yaml
	${NFPM} -f ${BUILD_DIR}/upstart_deb.yaml pkg -t $@

clean:
	rm -rf $(BUILD_DIR)

stage-deb-exe-local: build
	mkdir -p ${DEB_BUILD_DIR}/${PKGDIR_BIN}
	cp -p ${BUILD_DIR}/${APP_NAME}_${OS}_${ARCH} ${DEB_BUILD_DIR}/${PKGDIR_BIN}/${EXE}

${DEB_BUILD_DIR}/${PKGDIR_BIN}/${EXE} :
	mkdir -p ${DEB_BUILD_DIR}/${PKGDIR_BIN}
	$(WGET) -q --no-use-server-timestamps -O $@ $(BIN_URL)
	chmod +x $@

${DEB_BUILD_DIR}/${UPSTART_CONF} : ${DEB_SRC_DIR}/service.upstart
${DEB_BUILD_DIR}/${APP_CFG} : ${SRC_DIR}/generic/sample.cfg
${DEB_BUILD_DIR}/${LOGROTATE_CFG} : ${SRC_DIR}/generic/logrotate.cfg
${DEB_BUILD_DIR}/${SYSTEMD_CONF} : ${GENERIC_SRC_DIR}/service.systemd
${DEB_BUILD_DIR}/${UPSTART_DEFAULT} : ${DEB_SRC_DIR}/upstart_default.cfg

# Regular package files
${DEB_BUILD_DIR}/${SYSTEMD_CONF} ${DEB_BUILD_DIR}/${UPSTART_DEFAULT} ${DEB_BUILD_DIR}/${LOGROTATE_CFG} ${DEB_BUILD_DIR}/${APP_CFG}  :
	mkdir -p $(@D)
	cp $< $@

# Executable package files
${DEB_BUILD_DIR}/${UPSTART_CONF} :
	mkdir -p $(@D)
	cp $< $@
	chmod +x $@

# .PHONY tells make to not expect these goals to be actual files produced by the rule
.PHONY: default repackage package package-debs package-repo-upload package-upload-deb package-debs-local reprepro-debs \
	clean clean-repos generate-mocks stage-deb-exe-local build test test-integrationcli coverage install-nfpm \
	generate-callgraphs regenerate-callgraphs clean-callgraphs
