import { Money, addMoney, money, multiplyMoney } from "./money";
import { TaxPolicy, calculateTax } from "./taxPolicy";

export type InvoiceLineInput = {
  description: string;
  category: string;
  quantity: number;
  unitPrice: Money;
};

export type InvoiceLine = InvoiceLineInput & {
  subtotal: Money;
  tax: Money;
  total: Money;
};

export type Invoice = {
  id: string;
  customerId: string;
  lines: InvoiceLine[];
  subtotal: Money;
  tax: Money;
  total: Money;
};

export function buildInvoice(
  id: string,
  customerId: string,
  lineInputs: InvoiceLineInput[],
  taxPolicy: TaxPolicy,
): Invoice {
  if (lineInputs.length === 0) {
    throw new Error("Invoice requires at least one line.");
  }

  const lines = lineInputs.map((line) => buildInvoiceLine(line, taxPolicy));
  const subtotal = addMoney(lines.map((line) => line.subtotal));
  const tax = addMoney(lines.map((line) => line.tax));

  return {
    id,
    customerId,
    lines,
    subtotal,
    tax,
    total: money(subtotal.cents + tax.cents),
  };
}

function buildInvoiceLine(input: InvoiceLineInput, taxPolicy: TaxPolicy): InvoiceLine {
  if (input.quantity <= 0) {
    throw new Error("Invoice line quantity must be positive.");
  }

  const subtotal = multiplyMoney(input.unitPrice, input.quantity);
  const tax = calculateTax(subtotal, input.category, taxPolicy);

  return {
    ...input,
    subtotal,
    tax,
    total: money(subtotal.cents + tax.cents),
  };
}

