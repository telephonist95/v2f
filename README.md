# ver2fal

Конвертер Yosys-нетлиста SystemVerilog в схему Falstad CircuitJS, УГО по ГОСТ 2.743-91 и временную диаграмму (VCD).

## Требования

`yosys`, `iverilog`, `vvp`, `go` (1.22+), `node`/`npm` в `$PATH`.

## Запуск (веб-приложение)

```sh
cd vkr && npm install && cd ..   # зависимости фронтенда (однократно)
make serve                       # сборка + сервер на http://127.0.0.1:8090
```

## Makefile

```sh
make            # конвертер + Falstad .txt для verilog/*.sv
make json       # verilog/*.sv -> verilog/*.json (synth.tcl)
make circuits   # verilog/*.json -> ./*.txt
make web        # фронтенд -> vkr/dist (после npm install)
make serve      # сборка всего + сервер на :8090
make clean      # удалить бинарник и сгенерированное
```

## CLI

```sh
go build -buildvcs=false -o ver2fal ./src

./ver2fal verilog/counter.json                          # вентильный уровень
./ver2fal -rtl verilog/counter.json                     # RTL
./ver2fal -serve -addr 127.0.0.1:8080 -static vkr/dist  # сервер на :8080
```

## Dev (hot reload)

```sh
go build -buildvcs=false -o ver2fal ./src
./ver2fal -serve -addr 127.0.0.1:8080 -static vkr/dist   # терминал 1
cd vkr && npm install && npm run dev                     # терминал 2 -> :5173
```

## Тесты

```sh
cd src && go test ./...
```

## Структура

```
src/        Go: парсер, размещение/трассировка, RTL-абстрактор, Falstad/ГОСТ, VCD
verilog/    примеры на SystemVerilog
synth.tcl   синтез Yosys (ограниченный набор ячеек)
Makefile    сборка, синтез, запуск
vkr/js/     фронтенд Vue 3
vkr/*.typ   исходник пояснительной записки (Typst)
```
