// 4-битный сумматор с переносом
module adder4 (
    input  logic [3:0] a,     // первое слагаемое
    input  logic [3:0] b,     // второе слагаемое
    input  logic       cin,   // входной перенос
    output logic [3:0] sum,   // сумма
    output logic       cout   // выходной перенос
);

    assign {cout, sum} = a + b + cin;

endmodule
