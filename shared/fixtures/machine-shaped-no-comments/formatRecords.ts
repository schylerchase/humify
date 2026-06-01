type RawRecord = Record<string, unknown>;

export function formatRecords(records: RawRecord[], config: Record<string, unknown>) {
  const output = [];
  const fields = ["id", "name", "email", "status"];
  const defaults = ["", "", "", "unknown"];

  for (const record of records) {
    const next: Record<string, unknown> = {};

    for (let index = 0; index < fields.length; index++) {
      const field = fields[index];
      const value = record[field];

      if (value === null || value === undefined) {
        next[field] = defaults[index];
      } else if (typeof value === "string") {
        next[field] = value.trim();
      } else {
        next[field] = value;
      }
    }

    if (config["metadata"] === true) {
      next["metadata"] = {
        date: new Date().toISOString(),
        type: "formatted",
      };
    }

    output.push(next);
  }

  return output;
}

