export interface SSEMessage {
  /** Event name, defaults to "message" per the SSE spec. */
  event: string;
  /** Data payload with multiple data lines joined by "\n". */
  data: string;
}

export type SSEMessageHandler = (message: SSEMessage) => void;

/**
 * Incremental text/event-stream parser.
 *
 * Feed it decoded string chunks as they arrive; it invokes `onMessage` once per
 * completed event. Handles CRLF line endings, `event:`/`data:` fields, and
 * comment lines (": keep-alive"). Unknown fields are ignored per spec.
 */
export function createSSEParser(onMessage: SSEMessageHandler): (chunk: string) => void {
  let buffer = '';
  let eventName = '';
  let dataLines: string[] = [];

  const reset = (): void => {
    eventName = '';
    dataLines = [];
  };

  const dispatch = (): void => {
    if (dataLines.length === 0 && eventName === '') {
      return;
    }
    onMessage({ event: eventName || 'message', data: dataLines.join('\n') });
    reset();
  };

  const processLine = (line: string): void => {
    if (line === '') {
      dispatch();
      return;
    }
    if (line.startsWith(':')) {
      return;
    }

    const colonIndex = line.indexOf(':');
    const field = colonIndex === -1 ? line : line.slice(0, colonIndex);
    let value = colonIndex === -1 ? '' : line.slice(colonIndex + 1);
    if (value.startsWith(' ')) {
      value = value.slice(1);
    }

    if (field === 'event') {
      eventName = value;
    } else if (field === 'data') {
      dataLines.push(value);
    }
    // id / retry and anything else is intentionally ignored.
  };

  return (chunk: string) => {
    buffer += chunk;
    let newlineIndex = buffer.indexOf('\n');
    while (newlineIndex !== -1) {
      let line = buffer.slice(0, newlineIndex);
      buffer = buffer.slice(newlineIndex + 1);
      if (line.endsWith('\r')) {
        line = line.slice(0, -1);
      }
      processLine(line);
      newlineIndex = buffer.indexOf('\n');
    }
  };
}
