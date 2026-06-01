type InvoiceLine = {
  description: string;
  amount: number;
  taxable: boolean;
};

type InvoiceSummary = {
  subtotal: number;
  tax: number;
  total: number;
};

const TAX_RATE = 0.0825;

export function summarizeInvoice(lines: InvoiceLine[]): InvoiceSummary {
  const subtotal = sumLineAmounts(lines);
  const tax = calculateTax(lines);

  return {
    subtotal,
    tax,
    total: subtotal + tax,
  };
}

function sumLineAmounts(lines: InvoiceLine[]): number {
  return lines.reduce((total, line) => total + line.amount, 0);
}

function calculateTax(lines: InvoiceLine[]): number {
  return lines
    .filter((line) => line.taxable)
    .reduce((tax, line) => tax + line.amount * TAX_RATE, 0);
}

