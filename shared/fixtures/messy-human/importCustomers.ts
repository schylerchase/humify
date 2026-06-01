type ImportRow = Record<string, string>;

type Customer = {
  name: string;
  email: string;
  active: boolean;
};

export async function importCustomers(rows: ImportRow[], db: any, logger: any) {
  let imported = 0;
  let skipped = 0;

  for (const row of rows) {
    if (!row.name || row.name.trim() === "") {
      logger.warn("Skipping row without customer name");
      skipped++;
      continue;
    }

    if (!row.email || !row.email.includes("@")) {
      logger.warn("Skipping row without valid customer email");
      skipped++;
      continue;
    }

    const customer: Customer = {
      name: row.name.trim(),
      email: row.email.trim().toLowerCase(),
      active: row.status !== "inactive",
    };

    const existing = await db.customers.findByEmail(customer.email);
    if (existing) {
      await db.customers.update(existing.id, customer);
    } else {
      await db.customers.create(customer);
    }

    imported++;
  }

  return { imported, skipped };
}

