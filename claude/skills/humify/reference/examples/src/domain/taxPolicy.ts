import { Money, money, multiplyMoney } from "./money";

export type TaxPolicy = {
  rate: number;
  taxableCategories: string[];
};

export function calculateTax(subtotal: Money, category: string, policy: TaxPolicy): Money {
  if (!policy.taxableCategories.includes(category)) {
    return money(0);
  }

  return multiplyMoney(subtotal, policy.rate);
}

