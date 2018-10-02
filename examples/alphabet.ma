; Generated using MSCR compiler version 0.1.2

JMP .mscr_init_main

0x4000 ; HSP

.mscr_data __LABEL_SET
0x0
0x0
0x48
0x65
0x6c
0x6c
0x6f
0x20
0x77
0x6f
0x72
0x6c
0x64
0x21
0x0
0x1

; MSCR initialization routine
.mscr_init_main __LABEL_SET
SET SP ; Stack
0x3FFE
SET H ; VarHeap
.mscr_code_end

CALL .mscr_init_userland ; Call program specific initialization

MOV 0 A
PUSH 0
CALL .mscr_function_var_main_params_2 ; Call userland main

HALT ; After execution, halt


; MSCR bootloader static value loader
.mscr_init_bootloader SETREG A 0x14 ; Data block end address
SETREG B 0x0003 ; Data start
SETREG C 0xD003 ; Start of readonly CFG region for bootloader ROM

.mscr_init_bootloader_loop_start __REG_ASSIGN
MEMR D C ; Read from ROM to regD
MEMW D B ; Write to RAM
INC C ; Increment read address
INC B ; Increment write address
EQ A D B ; Check if we reached end of data and jump accordingly
JMPNZ .mscr_init_bootloader_return D
JMP .mscr_init_bootloader_loop_start

.mscr_init_bootloader_return RET ; Return out


.mscr_init_userland __LABEL_SET
CALL .mscr_init_bootloader
RET ;Userland init end
.mscr_function_putchar_params_1 __LABEL_SET ; [Function (in func: putchar)]
SETREG G [{[ [2] ]}] ; [Function (in func: putchar)]
ADD G H H ; [Function (in func: putchar)]
MOV [{[ [vga_mem + cursorPositionX + (cursorPositionY * 98)] ]}] B ; [Variable (in func: putchar)]
MEMW A B ; [Body (in func: putchar)]
SETREG G [{[ [0] ]}] ; [Function (in func: )]
SUB H H G ; [Function (in func: )]
RET ; [Function (in func: )]
FAULT 0x0 ; Ending function: putchar [Function (in func: )]
.mscr_function_alphabet_params_0 __LABEL_SET ; [Function (in func: alphabet)]
SETREG G [{[ [1] ]}] ; [Function (in func: alphabet)]
ADD G H H ; [Function (in func: alphabet)]
MOV [{[ 65 ]}] A ; [Variable (in func: alphabet)]
.mscr_while_start__24_5_356 JMPEZ .mscr_while_end__24_5_356 [{[ [curChar <= 90] ]}] ; [WhileLoop (in func: alphabet)]
MOV A A ; [FunctionCall (in func: alphabet)]
CALL .mscr_function_putchar_params_1 ; [FunctionCall (in func: alphabet)]
MOV [{[ [curChar + (1)] ]}] A ; [Assignment (in func: alphabet)]
MOV [{[ [cursorPositionX + (1)] ]}] A ; [Assignment (in func: alphabet)]
JMP .mscr_while_start__24_5_356 ; [WhileLoop (in func: alphabet)]
.mscr_while_end__24_5_356 __LABEL_SET ; [WhileLoop (in func: alphabet)]
SETREG G [{[ [0] ]}] ; [Function (in func: )]
SUB H H G ; [Function (in func: )]
RET ; [Function (in func: )]
FAULT 0x0 ; Ending function: alphabet [Function (in func: )]
.mscr_function_main_params_2 __LABEL_SET ; [Function (in func: main)]
SETREG G [{[ [3] ]}] ; [Function (in func: main)]
ADD G H H ; [Function (in func: main)]
POP B ; [Function (in func: main)]
JMPEZ .mscr_cond_else__35_5_503 [{[ [argc != 0] ]}] ; [IfCondition (in func: main)]
HALT ; [BodyIf (in func: main)]
MOV [{[ 1 ]}] A ; [BodyIf (in func: main)]
SETREG G [{[ [3] ]}] ; [BodyIf (in func: main)]
SUB H H G ; [BodyIf (in func: main)]
RET ; [BodyIf (in func: main)]
JMP .mscr_cond_end__35_5_503 ; [BodyElse (in func: main)]
.mscr_cond_else__35_5_503 __LABEL_SET ; [BodyElse (in func: main)]
PUSH [{[ 6 ]}] ; [FunctionCall (in func: main)]
PUSH [{[ 5 ]}] ; [FunctionCall (in func: main)]
PUSH [{[ 4 ]}] ; [FunctionCall (in func: main)]
PUSH [{[ 3 ]}] ; [FunctionCall (in func: main)]
PUSH [{[ 2 ]}] ; [FunctionCall (in func: main)]
MOV [{[ 1 ]}] A ; [FunctionCall (in func: main)]
SETREG G 0x0
SUB H G G
STOR A G
CALL .mscr_function_testAdd_params_6 ; [FunctionCall (in func: main)]
MOV [{[ testAdd(29060,2,3,4,5,6) ]}] A ; [Assignment (in func: main)]
.mscr_cond_end__35_5_503 __LABEL_SET ; [IfCondition (in func: main)]
MOV [{[ _d(32902) ]}] E ; [Assignment (in func: main)]
.mscr_while_start__47_5_717 JMPEZ .mscr_while_end__47_5_717 [{[ [iteration < 5] ]}] ; [WhileLoop (in func: main)]
MOV [{[ [iteration + (1)] ]}] C ; [Assignment (in func: main)]
CALL .mscr_function_alphabet_params_0 ; [FunctionCall (in func: main)]
MOV [{[ [cursorPositionY + (1)] ]}] E ; [Assignment (in func: main)]
MOV [{[ 0 ]}] A ; [Assignment (in func: main)]
JMP .mscr_while_start__47_5_717 ; [WhileLoop (in func: main)]
.mscr_while_end__47_5_717 __LABEL_SET ; [WhileLoop (in func: main)]
HALT ; [Body (in func: main)]
MOV [{[ 0 ]}] A ; [Body (in func: main)]
SETREG G [{[ [3] ]}] ; [Body (in func: main)]
SUB H H G ; [Body (in func: main)]
RET ; [Body (in func: main)]
FAULT 0x0 ; Ending function: main [Function (in func: )]
.mscr_function_testAdd_params_6 __LABEL_SET ; [Function (in func: testAdd)]
SETREG G [{[ [6] ]}] ; [Function (in func: testAdd)]
ADD G H H ; [Function (in func: testAdd)]
POP B ; [Function (in func: testAdd)]
POP C ; [Function (in func: testAdd)]
POP D ; [Function (in func: testAdd)]
POP E ; [Function (in func: testAdd)]
SETREG G 0x0
SUB H G G
STOR A G
POP A ; [Function (in func: testAdd)]
MOV [{[ [a+b+c+d+e+f] ]}] A ; [Body (in func: testAdd)]
SETREG G [{[ [6] ]}] ; [Body (in func: testAdd)]
SUB H H G ; [Body (in func: testAdd)]
RET ; [Body (in func: testAdd)]
FAULT 0x0 ; Ending function: testAdd [Function (in func: )]
.mscr_code_end HALT