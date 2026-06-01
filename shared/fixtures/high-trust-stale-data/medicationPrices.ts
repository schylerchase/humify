type MedicationPrice = {
  name: string;
  priceUsd: number;
  source: string;
  lastUpdated: string;
};

export const curatedMedicationPrices: MedicationPrice[] = [
  {
    name: "Samplemed",
    priceUsd: 28.5,
    source: "public formulary snapshot",
    lastUpdated: "2023-01-01",
  },
];

export function renderPriceCard(price: MedicationPrice) {
  return {
    title: price.name,
    price: `$${price.priceUsd.toFixed(2)}`,
    badge: "Verified price",
    source: price.source,
  };
}
