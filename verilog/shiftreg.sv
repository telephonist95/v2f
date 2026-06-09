// 4-битный сдвиговый регистр
module shiftreg (
    input  logic       clk,     // тактовый сигнал
    input  logic       rst_n,   // синхронный сброс (активный низкий)
    input  logic       en,      // разрешение сдвига
    input  logic       din,     // входной бит
    output logic [3:0] dout     // содержимое регистра
);

    always_ff @(posedge clk) begin
        if (!rst_n)
            dout <= 4'd0;
        else if (en)
            dout <= {dout[2:0], din};
    end

endmodule
