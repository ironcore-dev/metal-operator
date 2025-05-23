FROM squidfunk/mkdocs-material:latest

LABEL project=metal_operator

WORKDIR /docs

COPY docs/requirements.txt requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

EXPOSE 9000

# Start development server by default
ENTRYPOINT ["mkdocs"]
CMD ["serve", "--dev-addr=0.0.0.0:9000"]
