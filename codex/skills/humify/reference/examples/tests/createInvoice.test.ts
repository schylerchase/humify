import { createInvoice } from "../src/application/createInvoice";
import { money } from "../src/domain/money";
import { InMemoryInvoiceRepository } from "../src/infrastructure/inMemoryInvoiceRepository";

async function createExampleInvoice() {
  const repository = new InMemoryInvoiceRepository();

  const invoice = await createInvoice(
    {
      invoiceId: "inv_001",
      customerId: "cust_001",
      lines: [
        {
          description: "Managed workstation",
          category: "service",
          quantity: 2,
          unitPrice: money(12500),
        },
      ],
    },
    {
      rate: 0.0825,
      taxableCategories: ["service"],
    },
    repository,
  );

  return { invoice, repository };
}

describe("createInvoice", () => {
  it("calculates subtotal, tax, and total before saving the invoice", async () => {
    const { invoice, repository } = await createExampleInvoice();

    expect(invoice.subtotal).toEqual(money(25000));
    expect(invoice.tax).toEqual(money(2063));
    expect(invoice.total).toEqual(money(27063));
    expect(repository.findById("inv_001")).toEqual(invoice);
  });

  it("rejects invoices without lines", async () => {
    const repository = new InMemoryInvoiceRepository();

    await expect(
      createInvoice(
        {
          invoiceId: "inv_empty",
          customerId: "cust_001",
          lines: [],
        },
        {
          rate: 0.0825,
          taxableCategories: ["service"],
        },
        repository,
      ),
    ).rejects.toThrow("Invoice requires at least one line.");
  });
});

