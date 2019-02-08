.PHONY: default install vm
default: mcpc-bootloader/bootloader.mif | install

vm: mcpc-bootloader/bootloader_tmp.mb | install
	mcpc vm mcpc-bootloader/bootloader_tmp.mb

install:
	go install mcpc.go

mcpc-bootloader/bootloader.mif: mcpc-bootloader/bootloader_tmp.mb
	srec_cat mcpc-bootloader/bootloader_tmp.mb -binary -o mcpc-bootloader/bootloader.mif -mif 16

mcpc-bootloader/bootloader_tmp.mb: mcpc-bootloader/*.mscr
	cd mcpc-bootloader; mcpc mscr ./entry.mscr ./bootloader_tmp.ma --bootloader
	sed -i '$$d' mcpc-bootloader/bootloader_tmp.ma # Remove last line
	cat mcpc-bootloader/asm.ma >> mcpc-bootloader/bootloader_tmp.ma # Append hand-crafted ASM
	mcpc assemble --library assembler-libs/base.mlib --library assembler-libs/sram.mlib --library assembler-libs/sram_paged.mlib mcpc-bootloader/bootloader_tmp.ma mcpc-bootloader/bootloader_tmp.mb
	rm mcpc-bootloader/bootloader_tmp.ma 
