DIST := dist
SOURCES := $(shell find . -type f -name '*.go')

VERSION := 0.1
BUILD := `git describe --tags --always | sed 's/-/+/' | sed 's/^v//'`
LDFLAGS := -X "main.appVersion=$(VERSION)" -X "main.appBuild=$(BUILD)"

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

.PHONY: generate-statics-sources
generate-statics-sources:
	@hash rice > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go get -u github.com/GeertJohan/go.rice; \
	fi
	go generate

.PHONY: install
install: $(wildcard *.go)
	go install -v -tags '$(TAGS)' -ldflags '-s -w $(LDFLAGS)'

.PHONY: build
build:
	go build .

JAVA_SOURCE := "http://ftp.us.debian.org/debian/pool/main/o/openjdk-8/openjdk-8-doc_8u181-b13-1_all.deb"
C_CPP_SOURCE := "http://upload.cppreference.com/mwiki/images/1/1d/html_book_20180311.tar.xz"
PASCAL_SOURCE := "https://downloads.sourceforge.net/project/freepascal/Documentation/3.0.4/doc-html.tar.gz"
PYTHON_2_SOURCE := "https://www.python.org/ftp/python/2.7.15/python2715.chm"
PYTHON_3_SOURCE := "https://www.python.org/ftp/python/3.7.0/python370.chm"
JS_SOURCE := "https://github.com/agibsonsw/AndySublime/blob/master/LanguageHelp/javascript.chm?raw=true"

define writeInfo
	printf "{\"name\": \"$(1)\", \"title\": \"$(2)\", \"index\": \"$(3)\"}" "" >> "info.json"
endef

define packCHM
	wget -nv -O $(2).chm $(1)
	7z x $(2).chm -o$(2)
	$(call writeInfo,$(2),$(3),$(4))
endef

.ONESHELL:
.PHONY: reference
reference:
	rm -rf reference.zip
	mkdir -p reference
	cd reference

	printf "[" >> info.json

	### JAVA:
	wget -nv -O java.deb $(JAVA_SOURCE)
	ar x java.deb
	tar -xf data.tar.xz
	mv usr/share/doc/openjdk-8-jre-headless/api java
	$(call writeInfo,java,Java,index.html)
	rm -rf control.tar.xz data.tar.xz debian-binary usr

	printf "," >> info.json

	### C/C++:
	wget -nv -O c_cpp.tar.xz $(C_CPP_SOURCE)
	tar -xf c_cpp.tar.xz reference
	mv reference c_cpp
	$(call writeInfo,c_cpp,C/C++,en/index.html)

	printf "," >> info.json

	### PASCAL:
	wget -nv -O pascal.tar.gz $(PASCAL_SOURCE)
	tar -xzf pascal.tar.gz
	mv doc pascal
	$(call writeInfo,pascal,Pascal,fpctoc.html)

	printf "," >> info.json

	### PYTHON 2:
	$(call packCHM,$(PYTHON_2_SOURCE),python2,"Python\ 2",index.html)

	printf "," >> info.json

	### PYTHON 3:
	$(call packCHM,$(PYTHON_3_SOURCE),python3,"Python\ 3",index.html)

	printf "," >> info.json

	### JS:
	$(call packCHM,$(JS_SOURCE),javascript,JavaScript,default.htm)

	printf "]" >> info.json

	### ZIP EVERYTHING:
	zip -9 -rq ../reference.zip info.json java c_cpp pascal python2 python3 javascript

	rm -r info.json java c_cpp pascal python2 python3 javascript
