type ReportRow = Record<string, string | number | boolean | null>;

export async function runReportWorkflow(rows: ReportRow[], db: any, renderer: any, logger: any) {
  const accepted: ReportRow[] = [];
  const rejected: ReportRow[] = [];

  for (const row of rows) {
    if (!row["accountId"]) {
      rejected.push(row);
      logger.warn("missing account id");
      continue;
    }

    if (row["disabled"] === true) {
      rejected.push(row);
      logger.warn("disabled account skipped");
      continue;
    }

    accepted.push({
      accountId: String(row["accountId"]).trim(),
      amount: Number(row["amount"] ?? 0),
      region: String(row["region"] ?? "unknown"),
    });
  }

  const enriched = [];
  for (const row of accepted) {
    const account = await db.accounts.findById(row["accountId"]);
    if (!account) {
      logger.warn("account not found");
      continue;
    }

    enriched.push({
      ...row,
      owner: account.owner,
      tier: account.tier,
    });
  }

  const totals: Record<string, number> = {};
  for (const row of enriched) {
    const region = String(row["region"]);
    totals[region] = (totals[region] ?? 0) + Number(row["amount"]);
  }

  const html = renderer.render({
    rows: enriched,
    totals,
    rejectedCount: rejected.length,
  });

  await db.reports.save({
    createdAt: new Date().toISOString(),
    html,
    rowCount: enriched.length,
    rejectedCount: rejected.length,
  });

  return {
    html,
    rowCount: enriched.length,
    rejectedCount: rejected.length,
  };
}

