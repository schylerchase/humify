export type Money = {
  cents: number;
  currency: "USD";
};

export function money(cents: number): Money {
  if (!Number.isInteger(cents)) {
    throw new Error("Money must be represented in whole cents.");
  }

  return { cents, currency: "USD" };
}

export function addMoney(values: Money[]): Money {
  return money(values.reduce((total, value) => total + value.cents, 0));
}

export function multiplyMoney(value: Money, multiplier: number): Money {
  return money(Math.round(value.cents * multiplier));
}

