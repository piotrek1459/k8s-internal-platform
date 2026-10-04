FROM debian:bookworm-slim

# Packages used by the university template (pdfLaTeX + BibTeX).
RUN apt-get -o Acquire::Retries=3 update \
    && apt-get -o Acquire::Retries=3 install -y --no-install-recommends \
        latexmk \
        lmodern \
        texlive-bibtex-extra \
        texlive-fonts-recommended \
        texlive-lang-english \
        texlive-lang-polish \
        texlive-latex-extra \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work

CMD ["latexmk", "-pdf", "-interaction=nonstopmode", "-halt-on-error", "-file-line-error", "-synctex=1", "-outdir=build", "main.tex"]
