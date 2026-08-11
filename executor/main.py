from urllib.parse import urlparse

from app.api.routes import create_app
from app.core.config import settings

app = create_app()


if __name__ == "__main__":
    import uvicorn

    endpoint = urlparse(settings.executor_endpoint)
    host = endpoint.hostname or "127.0.0.1"
    port = endpoint.port or (443 if endpoint.scheme == "https" else 80)
    uvicorn.run(app, host=host, port=port, reload=False)
