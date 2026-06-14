`timescale 1ns/1ps

module A_tb;

    // Signals
    logic clk;
    logic rst_n;
    logic a;
    logic y;

    // Instantiate UUT
    A uut (
        .clk(clk),
        .rst_n(rst_n),
        .a(a),
        .y(y)
    );

    // Clock
    initial begin
        clk = 0;
        forever #5 clk = ~clk;
    end

    initial begin
        $dumpfile("A.vcd");
        $dumpvars(0, A_tb);

        // Initialize
        a = 0;

        // Reset
        rst_n = 0;
        #20;
        rst_n = 1;

        // Stimulus
        #10000;

        $finish;
    end

endmodule
