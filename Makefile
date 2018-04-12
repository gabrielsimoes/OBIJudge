DIST := dist
SOURCES := $(shell find . -type f -name '*.go')

VERSION := 0.1
BUILD := `git describe --tags --always | sed 's/-/+/' | sed 's/^v//'`
LDFLAGS := -X "main.appVersion=$(VERSION)" -X "main.appBuild=$(BUILD)"
GOFLAGS := -i -v

.PHONY: all
all: build

.PHONY: clean
clean:
	go clean -i .
	rm -rf dist/ OBIJudge* node_modules rice-box.go
	gulp static:clean

.PHONY: static
static:
	node_modules/.bin/gulp static:build

.PHONY: generate
generate:
	@hash rice > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go get -u github.com/GeertJohan/go.rice; \
	fi
	go generate

.PHONY: install
install: $(wildcard *.go)
	go install -v -tags '$(TAGS)' -ldflags '-s -w $(LDFLAGS)'

.PHONY: build
build: $(EXECUTABLE)

$(EXECUTABLE): $(SOURCES)
	go build $(GOFLAGS) $(EXTRA_GOFLAGS) -tags '$(TAGS)' -ldflags '-s -w $(LDFLAGS)' -o $@

.PHONY: release
release: release-static generate release-dirs release-build

.PHONY: release-dirs
release-dirs:
	mkdir -p $(DIST)

.PHONY: release-static
release-static:
	NODE_ENV=production node_modules/.bin/gulp static:build

.PHONY: release-build
release-build:
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) get -u github.com/karalabe/xgo; \
	fi
	xgo -dest $(DIST) -tags 'netgo $(TAGS)' -ldflags '-linkmode external -extldflags "-static" $(LDFLAGS)' -targets 'linux/386,linux/amd64' -out OBIJudge-$(VERSION) .

JAVA_SOURCE := "http://ftp.us.debian.org/debian/pool/main/o/openjdk-8/openjdk-8-doc_8u162-b12-1_all.deb"
C_CPP_SOURCE := "http://upload.cppreference.com/mwiki/images/0/0f/html_book_20180311.zip"
PASCAL_SOURCE := "https://downloads.sourceforge.net/project/freepascal/Documentation/3.0.4/doc-html.tar.gz"
PYTHON_2_SOURCE := "https://www.python.org/ftp/python/2.7.14/python2714.chm"
PYTHON_3_SOURCE := "https://www.python.org/ftp/python/3.6.5/python365.chm"
JS_SOURCE := "https://github.com/agibsonsw/AndySublime/blob/master/LanguageHelp/javascript.chm?raw=true"

define writeInfo
	printf "%s name: $(1)\n  title: $(2)\n  index: $(3)\n" "-" >> "info.yml"
endef

define packCHM
	wget -nv -O $(2).chm $(1)
	7z x $(2).chm -o$(2)
	$(call writeInfo,$(2),$(3),$(4))
	rm -rf $(2).chm
endef

.ONESHELL:
reference:
	rm -rf reference reference.zip
	mkdir -p reference
	cd reference

	### JAVA:
	wget -nv -O java.deb $(JAVA_SOURCE)
	ar x java.deb
	tar -xf data.tar.xz
	mv usr/share/doc/openjdk-8-jre-headless/api java
	$(call writeInfo,java,Java,index.html)
	rm -rf java.deb control.tar.xz data.tar.xz debian-binary usr

	### C/C++:
	wget -nv -O c_cpp.tar.gz $(C_CPP_SOURCE)
	tar -xzf c_cpp.tar.gz reference
	mv reference c_cpp
	$(call writeInfo,c_cpp,C/C++,en/index.html)
	rm -rf c_cpp.tar.gz

	### PASCAL:
	wget -nv -O pascal.tar.gz $(PASCAL_SOURCE)
	tar -xzf pascal.tar.gz
	mv doc pascal
	$(call writeInfo,pascal,Pascal,fpctoc.html)
	rm -rf pascal.tar.gz

	### PYTHON 2:
	$(call packCHM,$(PYTHON_2_SOURCE),python2,"Python\ 2",index.html)

	### PYTHON 3:
	$(call packCHM,$(PYTHON_3_SOURCE),python3,"Python\ 3",index.html)

	### JS:
	$(call packCHM,$(JS_SOURCE),javascript,JavaScript,default.htm)

	### ZIP EVERYTHING:
	zip -9 -rq ../reference.zip ./*
	cd ..
	rm -rf reference
