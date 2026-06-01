import { Invoice, InvoiceLineInput, buildInvoice } from "../domain/invoice";
import { TaxPolicy } from "../domain/taxPolicy";

export type InvoiceRepository = {
  save(invoice: Invoice): Promise<void>;
};

export type CreateInvoiceRequest = {
  invoiceId: string;
  customerId: string;
  lines: InvoiceLineInput[];
};

export async function createInvoice(
  request: CreateInvoiceRequest,
  taxPolicy: TaxPolicy,
  repository: InvoiceRepository,
): Promise<Invoice> {
  const invoice = buildInvoice(
    request.invoiceId,
    request.customerId,
    request.lines,
    taxPolicy,
  );

  await repository.save(invoice);

  return invoice;
}

