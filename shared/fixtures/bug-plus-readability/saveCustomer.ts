type CustomerInput = {
  name: string;
  email: string;
};

type CustomerRepository = {
  save(customer: { name: string; email: string }): Promise<void>;
};

export async function saveCustomer(input: CustomerInput, repository: CustomerRepository) {
  const normalizedName = input.name.trim();
  const normalizedEmail = input.email.trim();

  if (!normalizedName) {
    throw new Error("Customer name is required.");
  }

  if (!normalizedEmail.includes("@")) {
    throw new Error("Customer email is invalid.");
  }

  await repository.save({
    name: normalizedName,
    email: input.email,
  });
}

