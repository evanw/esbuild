package js_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

var jsCode = `
import { useState, useEffect } from 'react';
import { debounce } from './utils';

const DEFAULT_OPTIONS = { timeout: 1000, retries: 3 };

class EventEmitter {
  #listeners = new Map();

  on(event, callback) {
    if (!this.#listeners.has(event)) {
      this.#listeners.set(event, []);
    }
    this.#listeners.get(event).push(callback);
    return this;
  }

  emit(event, ...args) {
    for (const cb of this.#listeners.get(event) ?? []) {
      cb(...args);
    }
  }
}

async function fetchData(url, { timeout, retries } = DEFAULT_OPTIONS) {
  for (let i = 0; i < retries; i++) {
    try {
      const controller = new AbortController();
      const id = setTimeout(() => controller.abort(), timeout);
      const response = await fetch(url, { signal: controller.signal });
      clearTimeout(id);
      if (!response.ok) throw new Error(` + "`Status: ${response.status}`" + `);
      return await response.json();
    } catch (err) {
      if (i === retries - 1) throw err;
      await new Promise(r => setTimeout(r, 100 * (i + 1)));
    }
  }
}

const processItems = (items) => {
  const [first, ...rest] = items;
  return rest.reduce((acc, item) => ({
    ...acc,
    [item.id]: { ...item, processed: true },
  }), { [first.id]: { ...first, processed: true } });
};

export { EventEmitter, fetchData, processItems };
`

var tsCode = `
interface HttpRequest<T = unknown> {
  url: string;
  method: 'GET' | 'POST' | 'PUT' | 'DELETE';
  body?: T;
  headers: Record<string, string>;
}

interface HttpResponse<T> {
  status: number;
  data: T;
  headers: Map<string, string>;
}

type RequestHandler<Req, Res> = (req: HttpRequest<Req>) => Promise<HttpResponse<Res>>;

enum LogLevel {
  Debug = 'debug',
  Info = 'info',
  Warn = 'warn',
  Error = 'error',
}

class Logger {
  private level: LogLevel;
  private static instance: Logger | null = null;

  private constructor(level: LogLevel) {
    this.level = level;
  }

  static getInstance(level: LogLevel = LogLevel.Info): Logger {
    if (!Logger.instance) {
      Logger.instance = new Logger(level);
    }
    return Logger.instance;
  }

  log<T extends object>(level: LogLevel, message: string, context?: T): void {
    console.log(JSON.stringify({ level, message, ...context }));
  }
}

async function handleRequest<Req, Res>(
  handler: RequestHandler<Req, Res>,
  request: HttpRequest<Req>,
): Promise<HttpResponse<Res>> {
  const logger = Logger.getInstance();
  logger.log(LogLevel.Info, 'Handling request', { url: request.url });
  return handler(request);
}

const identity = <T>(value: T): T => value;

export { Logger, LogLevel, handleRequest, identity };
export type { HttpRequest, HttpResponse, RequestHandler };
`

var jsxCode = `
import { useState, useEffect, useCallback, memo } from 'react';

function UserAvatar({ src, alt, size = 40 }) {
  return (
    <img
      className="avatar"
      src={src}
      alt={alt}
      width={size}
      height={size}
      style={{ borderRadius: '50%' }}
    />
  );
}

const UserCard = memo(function UserCard({ user, onSelect }) {
  const [expanded, setExpanded] = useState(false);

  const handleClick = useCallback(() => {
    setExpanded(prev => !prev);
    onSelect?.(user.id);
  }, [user.id, onSelect]);

  return (
    <div className={'card' + (expanded ? ' expanded' : '')} onClick={handleClick}>
      <UserAvatar src={user.avatar} alt={user.name} />
      <div className="info">
        <h3>{user.name}</h3>
        {user.role && <span className="badge">{user.role}</span>}
      </div>
      {expanded && (
        <div className="details">
          <p>{user.bio || 'No bio available'}</p>
          <ul>
            {user.skills.map((skill, i) => (
              <li key={i}>{skill}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
});

function UserList({ users }) {
  const [selected, setSelected] = useState(null);

  return (
    <>
      <h2>Users ({users.length})</h2>
      <div className="user-list">
        {users.length > 0
          ? users.map(user => (
              <UserCard key={user.id} user={user} onSelect={setSelected} />
            ))
          : <p>No users found</p>
        }
      </div>
      {selected && <p>Selected: {selected}</p>}
    </>
  );
}

export { UserList, UserCard, UserAvatar };
`

func BenchmarkParseJS(b *testing.B) {
	b.ReportAllocs()
	options := config.Options{
		OmitRuntimeForTests: true,
	}
	source := test.SourceForTest(jsCode)
	opts := OptionsFromConfig(&options)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		Parse(log, source, opts)
	}
}

func BenchmarkParseTypeScript(b *testing.B) {
	b.ReportAllocs()
	options := config.Options{
		OmitRuntimeForTests: true,
		TS: config.TSOptions{
			Parse: true,
		},
	}
	source := test.SourceForTest(tsCode)
	opts := OptionsFromConfig(&options)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		Parse(log, source, opts)
	}
}

func BenchmarkParseJSX(b *testing.B) {
	b.ReportAllocs()
	options := config.Options{
		OmitRuntimeForTests: true,
		JSX: config.JSXOptions{
			Parse: true,
		},
	}
	source := test.SourceForTest(jsxCode)
	opts := OptionsFromConfig(&options)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		Parse(log, source, opts)
	}
}
