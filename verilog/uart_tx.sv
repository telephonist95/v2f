// UART Transmitter (8N1)
// Baud rate = clk / (DIVISOR+1).
// Load data byte when start=1 and ready=1.
// tx_out: serial output, idle high.
module uart_tx #(
    parameter DIVISOR = 0  // 1 clock per bit for Falstad
)(
    input  logic       clk,
    input  logic       rst_n,
    input  logic       start,
    input  logic [7:0] data,
    output logic       tx_out,
    output logic       ready
);

    typedef enum logic [1:0] {
        IDLE  = 2'd0,
        START = 2'd1,
        DATA  = 2'd2,
        STOP  = 2'd3
    } state_t;

    state_t state, next_state;

    logic [7:0] shift_reg;
    logic [2:0] bit_cnt;
    logic [1:0] baud_cnt;   // counts 0..DIVISOR
    logic       baud_tick;

    // Baud rate divider
    assign baud_tick = (baud_cnt == DIVISOR[1:0]);

    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n)
            baud_cnt <= '0;
        else if (state == IDLE || baud_tick)
            baud_cnt <= '0;
        else
            baud_cnt <= baud_cnt + 1;
    end

    // FSM register
    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n)
            state <= IDLE;
        else
            state <= next_state;
    end

    // FSM next-state logic
    always_comb begin
        next_state = state;
        case (state)
            IDLE:  if (start) next_state = START;
            START: if (baud_tick) next_state = DATA;
            DATA:  if (baud_tick && bit_cnt == 3'd7) next_state = STOP;
            STOP:  if (baud_tick) next_state = IDLE;
        endcase
    end

    // Shift register and bit counter
    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n) begin
            shift_reg <= '0;
            bit_cnt   <= '0;
        end else if (state == IDLE && start) begin
            shift_reg <= data;
            bit_cnt   <= '0;
        end else if (state == DATA && baud_tick) begin
            shift_reg <= {1'b0, shift_reg[7:1]};
            bit_cnt   <= bit_cnt + 1;
        end
    end

    // Output logic
    always_comb begin
        case (state)
            IDLE:  tx_out = 1'b1;
            START: tx_out = 1'b0;
            DATA:  tx_out = shift_reg[0];
            STOP:  tx_out = 1'b1;
        endcase
    end

    assign ready = (state == IDLE);

endmodule
