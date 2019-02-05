# Internal M-Script compiler documentation

Register allocation:
A: Free, return value
B: Free
C: Free
D: Free
E: calc, return addr temp
F: calc out, stack staging, calc
G: MSCR Scratch
H: VarHeap pointer


Memory assignment:
0x0-0x2 ... Init JMP
0x3     ... Heap Start Pointer
0x4-    ... Data
   -    ... VarHeap
HSP-    ... Heap
...
(no VarHeap/stack collision protection as of yet!)
...
<-0x7FFF ... Stack (downward)


Function calling:
Parameters:
1: Register A
2-n: Stack

Function scoped variables: VarHeap


## Meta-Assembly-only commands

__CLEARSCOPE: resets scope information from here on out (does *not* generate output ASM)
__ASSUMESCOPE: assumes variable cmd.scopeAnnotationName is in cmd.scopeAnnotationRegister (dirty, does *not* generate output ASM) from here on out
__FLUSHSCOPE: saves all variables and globals checked out as dirty back to memory
__FLUSHGLOBALS: saves all globals checked out as dirty back to memory
__FORCESCOPE: forces variable cmd.scopeAnnotationName to be checked out into cmd.scopeAnnotationRegister, eviciting or overwriting whatever was checked out there previously
__SET_DIRECT: marks cmd.scopeAnnotationName as directly assigned variable, thus forcing it to be written to memory after every write access
__EVICT: forcibly evicts cmd.scopeAnnotationRegister (but leaves non-dirty checkout marker)