# MCPC-Software

This repository contains the software side of the of the [MCPC](https://github.com/PiMaker/MCPC-Hardware) project - mainly, the `mcpc` command line toolchain for building .ma and .mscr files and debugging them via a built-in MCPC-emulator (dubbed the "VM"), as well as the MCPC bootloader.

This repository is a Go package, you can install it via `go get`. Alternatively, issuing `make install` from the repository's root will install the `mcpc` toolchain application in your local environment.

Call `mcpc --help` for usage notes.

# License

The MCPC project is licensed under GPLv3. See `LICENSE` file for more information.

### Attributions

* The dijkstra-shunting-yard shell library in `mscr/dijkstra-shunting-yard` is licensed as GPLv2. The appropriate license can be found in the aforementioned folder.
* The GPP preprocessor application is licensed under the GNU LGPL. Read more at [GPP's website](https://logological.org/gpp).

#### Go Packages
* github.com/alecthomas/participle (MIT)
* github.com/mileusna/conditional (MIT)
* github.com/davecgh/go-spew (ISC)
* github.com/logrusorgru/aurora (WTFPL)
