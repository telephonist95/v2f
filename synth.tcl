# Синтез для ver2fal: только поддерживаемые ячейки
# Использование: SRC=file.sv TOP=module OUT=file.json yosys -c synth.tcl

yosys read_verilog -sv $::env(SRC)
yosys synth -top $::env(TOP) -noabc
yosys dfflegalize \
    -cell {$_DFF_P_}      0 \
    -cell {$_DFF_PN0_}    0 \
    -cell {$_DFFE_PN0P_}  0 \
    -cell {$_SDFFE_PN0P_} 01
yosys abc -g {AND,NAND,OR,NOR,XOR,XNOR,ANDNOT,ORNOT,MUX}
yosys clean
yosys write_json $::env(OUT)
