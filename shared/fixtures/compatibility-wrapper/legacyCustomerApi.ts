import { importCustomers } from "../messy-human/importCustomers";

type ImportRow = Record<string, string>;

export async function processCustomerData(rows: ImportRow[], db: unknown, logger: unknown) {
  return importCustomers(rows, db, logger);
}

export const legacyCustomerImport = processCustomerData;

