.PHONY: default install vm go-restore clean
default: build/bootloader.mif

vm: build/bootloader_tmp.mb
	mcpc vm build/bootloader_tmp.mb

install: go-restore
	go install mcpc.go

go-restore:
	go get -v

go-update:
	go get -v -u

clean:
	rm -rf build

test: install
	mcpc autotest tests --library assembler-libs/base.mlib --library assembler-libs/sram.mlib --library assembler-libs/sram_paged.mlib

build/bootloader.mif: build/bootloader_tmp.mb
	# Create mif file for Verilog
	srec_cat build/bootloader_tmp.mb -binary -o build/bootloader.mif -mif 16

build/bootloader_tmp.mb: mcpc-bootloader/*.mscr install
	mkdir -p build
	cd mcpc-bootloader; mcpc mscr ./entry.mscr ../build/bootloader_tmp.ma --bootloader
	sed -i '$$d' build/bootloader_tmp.ma # Remove last line
	cat mcpc-bootloader/asm.ma >> build/bootloader_tmp.ma # Append hand-crafted ASM
	mcpc assemble --debug-symbols --library assembler-libs/base.mlib --library assembler-libs/sram.mlib --library assembler-libs/sram_paged.mlib build/bootloader_tmp.ma build/bootloader_tmp.mb

