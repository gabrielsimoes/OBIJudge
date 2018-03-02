SHELL := /bin/bash
SOURCES := $(shell find . -type f -name '*.go')
BINARY := obijudge

VERSION := 0.1
BUILD := `git rev-parse HEAD`
LDFLAGS=-ldflags "-X=main.Version=$(VERSION) -X=main.Build=$(BUILD)"

.PHONY: install clean pack reference
.DEFAULT_GOAL: $(BINARY)

$(BINARY): $(SOURCES)
	@go build $(LDFLAGS) -o $(BINARY)

install:
	@go install $(LDFLAGS)

clean:
	@rm -rf $(BINARY) "rice-box.go" "database.db"

pack: clean
	@rice embed-go
	@$(BINARY) builddb

TABLE_32 := "https://raw.githubusercontent.com/torvalds/linux/master/arch/x86/entry/syscalls/syscall_32.tbl"
TABLE_64 := "https://raw.githubusercontent.com/torvalds/linux/master/arch/x86/entry/syscalls/syscall_64.tbl"

.SILENT:
define download_syscalls_script =
	regex_comment="^# ([0-9]+)\ "
	regex_syscall="^([0-9]+)\s+(common|64|x32|i386)\s+([0-9a-z_]+)"

	let i=0
	while read -r line || [[ -n "$line" ]]; do
		if [[ $line =~ $regex_comment ]]; then
			num="${BASH_REMATCH[1]}"
			echo "{sys_none, NULL}, // $num" >> syscalls_i386.h
		elif [[ $line =~ $regex_syscall ]]; then
			num="${BASH_REMATCH[1]}"
			abi="${BASH_REMATCH[2]}"
			name="${BASH_REMATCH[3]}"
			echo "{sys_$name, \"$name\"}, // $num" >> syscalls_i386.h
			echo "  sys_$name," >> syscalls_tab_pre
		fi
	done < syscall_32.tbl

	let i=0
	while read -r line || [[ -n "$line" ]]; do
		if [[ $line =~ $regex_syscall ]]; then
			num="${BASH_REMATCH[1]}"
			abi="${BASH_REMATCH[2]}"
			name="${BASH_REMATCH[3]}"

			let i++
			if [[ $i != $num ]]; then let i--; fi

			while [[ $i -lt $num ]]; do
				echo "{sys_none, NULL}, // $i" >> syscalls_x32.h
				echo "{sys_none, NULL}, // $i" >> syscalls_x86_64.h
				let i++
			done

			echo "  sys_$name," >> syscalls_tab_pre
			if [[ $abi == "i386" ]]; then
				echo "{sys_$name, \"$name\"}, // $num" >> syscalls_i386.h
			elif [[ $abi == "common" ]]; then
				echo "{sys_$name, \"$name\"}, // $num" >> syscalls_x32.h
				echo "{sys_$name, \"$name\"}, // $num" >> syscalls_x86_64.h
			elif [[ $abi == "x32" ]]; then
				echo "{sys_$name, \"$name\"}, // $num" >> syscalls_x32.h
				echo "{sys_none, NULL}, // $num" >> syscalls_x86_64.h
			elif [[ $abi == "64" ]]; then
				echo "{sys_none, NULL}, // $num" >> syscalls_x32.h
				echo "{sys_$name, \"$name\"}, // $num" >> syscalls_x86_64.h
			fi
		fi
	done < syscall_64.tbl

	echo "#ifndef SYSCALL_TAB_H" > syscalls_tab.h
	echo "#define SYSCALL_TAB_H" >> syscalls_tab.h
	echo "typedef enum SYSCALL {" >> syscalls_tab.h
	echo "  sys_none," >> syscalls_tab.h
	sort syscalls_tab_pre | uniq >> syscalls_tab.h
	echo "  NUM_SYSCALLS" >> syscalls_tab.h
	echo "} SYSCALL;" >> syscalls_tab.h
	echo "#endif // SYSCALL_TAB_H" >> syscalls_tab.h
	rm syscalls_tab_pre
endef

.ONESHELL:
download_syscalls:
	rm -f {syscalls_tab,syscalls_{i386,x32,x86_64}}.h
	wget -nv $(TABLE_32)
	wget -nv $(TABLE_64)
	echo "Parsing headers..."
	$(value download_syscalls_script)
	rm -r syscall_{32,64}.tbl

JAVA_SOURCE := "http://ftp.us.debian.org/debian/pool/main/o/openjdk-8/openjdk-8-doc_8u151-b12-1_all.deb"
C_CPP_SOURCE := "http://upload.cppreference.com/mwiki/images/3/37/html_book_20170409.tar.gz"
PASCAL_SOURCE := "https://downloads.sourceforge.net/project/freepascal/Documentation/3.0.2/doc-html.tar.gz"
PYTHON_2_SOURCE := "https://www.python.org/ftp/python/2.7.14/python2714.chm"
PYTHON_3_SOURCE := "https://www.python.org/ftp/python/3.6.3/python363.chm"
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

