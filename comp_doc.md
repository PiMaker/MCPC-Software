# Internal M-Script compiler documentation

Register allocation:
A: Free, global temp, return value
B: Free, global temp
C: Free, global temp
D: Free, calc 1
E: Free, calc 2
F: calc out, stack staging
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
<-0x3FFE ... Stack (downward)
0x3FFF ... Reserved/Scratch area


Function calling:
Parameters:
1: Register A
2-n: Stack

Function scoped variables: VarHeap
