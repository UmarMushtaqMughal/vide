`timescale 1ns/1ps

module counter_tb;

    // Signals
    logic clk;
    logic rst_n;
    logic [3:0] count;

    // Instantiate UUT
    counter uut (
        .clk(clk),
        .rst_n(rst_n),
        .count(count)
    );

    // Clock
    initial begin
        clk = 0;
        forever #5 clk = ~clk;
    end

    initial begin
        $dumpfile("counter.vcd");
        $dumpvars(1, counter_tb);

        // Reset
        rst_n = 0;
        #20;
        rst_n = 1;

        // Stimulus
        #10000;

        $finish;
    end

endmodule
