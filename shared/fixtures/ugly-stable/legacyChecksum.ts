export function legacyChecksum(value: string): number {
  let result = 0;
  for (let i = 0; i < value.length; i++) {
    result = (result + value.charCodeAt(i) * 31) % 65535;
  }
  return result;
}

