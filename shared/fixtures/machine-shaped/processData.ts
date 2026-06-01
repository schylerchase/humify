export function processData(data: any[], options: any) {
  const results: any[] = [];

  for (let i = 0; i < data.length; i++) {
    const item = data[i];

    // Check if item exists
    if (item !== null && item !== undefined) {
      // Create processed item
      const processedItem: any = {};

      // Process id field
      if (item.id !== null && item.id !== undefined) {
        processedItem.id = item.id;
      } else {
        processedItem.id = "";
      }

      // Process name field
      if (item.name !== null && item.name !== undefined) {
        processedItem.name = String(item.name).trim();
      } else {
        processedItem.name = "";
      }

      // Process status field
      if (item.status !== null && item.status !== undefined) {
        processedItem.status = item.status;
      } else {
        processedItem.status = "unknown";
      }

      // Process options
      if (options !== null && options !== undefined) {
        if (options.includeMetadata === true) {
          processedItem.metadata = {
            processedAt: new Date().toISOString(),
            source: "system",
          };
        }
      }

      results.push(processedItem);
    }
  }

  return results;
}

