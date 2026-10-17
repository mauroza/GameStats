# Contexto de build: raíz del repositorio.
FROM eclipse-temurin:21-jdk AS build
ARG SBT_VERSION=1.10.7
RUN curl -fsSL "https://github.com/sbt/sbt/releases/download/v${SBT_VERSION}/sbt-${SBT_VERSION}.tgz" \
    | tar xz -C /opt
ENV PATH=/opt/sbt/bin:$PATH
WORKDIR /src
# Dependencias primero para aprovechar la caché de capas.
COPY analytics/build.sbt ./
COPY analytics/project/build.properties analytics/project/plugins.sbt ./project/
RUN sbt -batch update
COPY analytics/src ./src
RUN sbt -batch test assembly

FROM eclipse-temurin:21-jre
RUN useradd -r app && mkdir -p /data && chown app /data
WORKDIR /app
COPY --from=build /src/target/scala-3.3.4/analytics.jar /app/analytics.jar
ENV PORT=9000 DATA_DIR=/data
USER app
VOLUME /data
EXPOSE 9000
HEALTHCHECK --interval=5s --timeout=3s --retries=20 \
  CMD curl -fs http://localhost:9000/health || exit 1
ENTRYPOINT ["java", "-jar", "/app/analytics.jar"]
