# Engineering thesis

The English title is set in `main.tex`. Fill in the author details, student ID,
supervisor, diploma path, department, and Polish title.

## Building the PDF

From the repository root:

```bash
./scripts/build-book.sh
```

From this directory, you can run `make`. Both commands use the same script and
write the result to `build/main.pdf`. The `build/` directory also contains
auxiliary files and logs; delete it to perform a clean build. The original
`main.pdf` remains an older preview of the template.

The build uses pdfLaTeX and BibTeX, matching the template's active bibliography
configuration. pdfLaTeX does not require the Calibri font used by the XeLaTeX
variant. `latexmk` automatically repeats the passes needed for the bibliography,
numbering, and lists. A LaTeX error stops the script with a nonzero exit code;
details are available in `build/main.log`.

## Where to write

| File / directory | Contents |
| --- | --- |
| `main.tex` | Thesis details, titles, and chapter order |
| `chapters/00.tex` | Abstracts and keywords |
| `chapters/01.tex` | Introduction, objectives, and scope |
| `chapters/02.tex` | Problem analysis and review of existing solutions |
| `chapters/03.tex` | Requirements and tools |
| `chapters/04.tex` | External specification and usage |
| `chapters/05.tex` | Design and implementation details |
| `chapters/06.tex` | Verification, testing, and results |
| `chapters/07.tex` | Summary and conclusions |
| `chapters/08.tex`–`10.tex` | Template appendices |
| `biblio/biblio.bib` | Bibliography entries |
| `graf/` | Figures and diagrams |
| `config/my-settings.tex` | Custom macros and additional packages |

Replace the template's sample content and references with your own material.
The formatting settings in `config/settings.tex` and the title page layout in
`config/titlepage.tex` come from the supplied template. Its front matter includes
fields for both English and Polish abstracts, titles, and keywords.

See [docs/project-outline.md](../docs/project-outline.md) for the project concept
and proposed implementation stages.
