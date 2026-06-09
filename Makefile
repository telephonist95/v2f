YOSYS   ?= yosys
VER2FAL := ./ver2fal
GOENV   ?= GOCACHE=/tmp/ver2fal-gocache
GOBUILD ?= go build -buildvcs=false

DESIGNS := counter adder4 mux4 shiftreg uart_tx
JSONS   := $(patsubst %,verilog/%.json,$(DESIGNS))
CIRCUITS:= $(patsubst %,%.txt,$(DESIGNS))

.PHONY: all clean json circuits web serve

all: circuits

# Синтез Verilog -> Yosys JSON (ячейки ограничены через synth.tcl)
json: $(JSONS)

verilog/counter.json: verilog/counter_synth.sv synth.tcl
	SRC=$< TOP=counter OUT=$@ $(YOSYS) -c synth.tcl

verilog/%.json: verilog/%.sv synth.tcl
	SRC=$< TOP=$* OUT=$@ $(YOSYS) -c synth.tcl

# Сборка конвертера
$(VER2FAL): $(wildcard src/*.go)
	$(GOENV) $(GOBUILD) -o $@ ./src

# Сборка веб-интерфейса
web:
	cd vkr && npm run build

# Локальный запуск веб-приложения и API
serve: $(VER2FAL) web
	$(VER2FAL) -serve -addr 127.0.0.1:8090 -static vkr/dist

# Конвертация JSON -> Falstad circuit text
circuits: $(CIRCUITS)

%.txt: verilog/%.json $(VER2FAL)
	$(VER2FAL) $< > $@

clean:
	rm -f $(VER2FAL) $(CIRCUITS) $(JSONS)
