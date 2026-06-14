`timescale 1ns/1ps

module A (
    input logic clk,
    input logic rst_n,
    // Add ports here (e.g. input logic [7:0] data)
    input logic a,
    output logic y
);
    assign y = a;
    // Logic here

endmodule
