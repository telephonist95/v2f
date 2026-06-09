// Модуль счётчика
module counter (
    input  logic       clk,      // тактовый сигнал
    input  logic       rst_n,    // синхронный сброс (активный низкий уровень)
    input  logic       en,       // разрешение счёта
    output logic [3:0] count     // текущее значение счётчика
);

    // Синхронная логика: по положительному фронту тактового сигнала
    always_ff @(posedge clk) begin
        if (!rst_n)          // если сброс активен (rst_n = 0)
            count <= 4'd0;    // сбросить счётчик в 0
        else if (en)          // иначе если разрешён счёт
            count <= count + 1'b1; // увеличить на 1
        // иначе (en = 0) значение сохраняется
    end

endmodule

// Тестовый модуль (testbench)
module tb_counter;

    logic       clk;
    logic       rst_n;
    logic       en;
    logic [3:0] count;

    // Генерация тактового сигнала с периодом 10 единиц времени
    initial begin
        clk = 0;
        forever #5 clk = ~clk; // период 10
    end

    // Подача стимулов
    initial begin
       $dumpfile("counter_tb.vcd"); // Имя файла для波形
       $dumpvars(0, tb_counter);    // Сохранять все сигналы в модуле tb_counter и ниже
        // Инициализация сигналов
        rst_n = 0;  // активный сброс
        en    = 0;

        // Даём пройти нескольким тактам со сбросом
        #20;
        rst_n = 1;  // снимаем сброс

        // Разрешаем счёт на несколько тактов
        en = 1;
        #50;

        // Запрещаем счёт
        en = 0;
        #20;

        // Снова разрешаем
        en = 1;
        #30;

        // Подаём сброс во время работы
        rst_n = 0;
        #10;
        rst_n = 1;
        #20;

        // Завершение симуляции
        $finish;
    end

    // Мониторинг изменений
    initial begin
        $monitor("Time = %0t, rst_n = %b, en = %b, count = %d", $time, rst_n, en, count);
    end

    // Подключаем проверяемый модуль
    counter u_counter (
        .clk   (clk),
        .rst_n (rst_n),
        .en    (en),
        .count (count)
    );

endmodule
