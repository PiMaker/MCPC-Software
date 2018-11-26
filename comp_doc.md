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
