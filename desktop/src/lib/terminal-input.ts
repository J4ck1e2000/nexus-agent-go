const MAX_INPUT_BYTES = 16 * 1024;

/** Keep large pastes within the IPC byte limit, without splitting Unicode. */
export function splitTerminalInput(data: string): string[] {
  const chunks: string[] = [];
  const encoder = new TextEncoder();
  let chunk = '';
  let bytes = 0;
  for (const character of data) {
    const size = encoder.encode(character).length;
    if (bytes + size > MAX_INPUT_BYTES) {
      chunks.push(chunk);
      chunk = '';
      bytes = 0;
    }
    chunk += character;
    bytes += size;
  }
  if (chunk) chunks.push(chunk);
  return chunks;
}
