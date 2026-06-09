// 4-в-1 мультиплексор (4 однобитных входа, 2-битный селектор)
module mux4 (
    input  logic [3:0] d,     // входные данные
    input  logic [1:0] sel,   // селектор
    output logic       y      // выход
);

    always_comb begin
        case (sel)
            2'd0: y = d[0];
            2'd1: y = d[1];
            2'd2: y = d[2];
            2'd3: y = d[3];
        endcase
    end

endmodule
