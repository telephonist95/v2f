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
