# MCPC-Software

This repository contains the software side of the of the [MCPC](https://github.com/PiMaker/MCPC-Hardware) project - mainly, the `mcpc` command line toolchain for building .ma and .mscr files and debugging them via a built-in MCPC-emulator dubbed the "VM", as well as the MCPC bootloader.

This repository is a Go package, you can install it via `go get`. Alternatively, issuing `make install` from the repositories root will install the `mcpc` toolchain application in your local environment.

Call `mcpc --help` for usage notes.
