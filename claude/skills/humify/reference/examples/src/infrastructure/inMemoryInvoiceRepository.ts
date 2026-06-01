import { InvoiceRepository } from "../application/createInvoice";
import { Invoice } from "../domain/invoice";

export class InMemoryInvoiceRepository implements InvoiceRepository {
  private readonly invoices = new Map<string, Invoice>();

  async save(invoice: Invoice): Promise<void> {
    this.invoices.set(invoice.id, invoice);
  }

  findById(id: string): Invoice | undefined {
    return this.invoices.get(id);
  }
}

