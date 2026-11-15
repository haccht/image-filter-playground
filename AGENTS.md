# Image Filter Playground — Agent Instructions

## Project Goal

Build a lightweight image processing web application running on Akamai Functions.

The application allows users to upload images, apply visual filters, preview results, and download processed images.

The primary purpose is to demonstrate practical image processing using Go, WebAssembly, and Akamai Functions.

## Technology

- Backend: Go
- Runtime: Akamai Functions / Spin / WebAssembly
- Frontend: Vanilla JavaScript, HTML, CSS
- Testing: Go standard testing framework
- Version control: Git

Prefer standard libraries and minimal dependencies.

## Development Principles

1. Keep the implementation simple and maintainable.
2. Prioritize Akamai Functions compatibility.
3. Verify SDK, toolchain, and runtime compatibility against official documentation before implementation.
4. Never assume that a native Go executable will work as a Spin component.
5. Separate image processing logic from HTTP handling.
6. Avoid unnecessary frameworks, databases, and external services.
7. Implement features incrementally.
8. Keep changes small and independently testable.

## Runtime Constraints

Design for resource-constrained edge execution.

Initial limits:

- Maximum processed image dimensions: 1024px longest side (aspect-ratio preserving auto-resize from sources up to 4096px / 1,048,576 total pixels).
- Maximum uploaded file size: 2 MiB (with a 4 MiB conservative decode allocation budget).
- Supported input formats: JPEG and PNG.
- Output format: JPEG (quality 85).
- Supported filters: Grayscale (`grayscale`), Pixelate (`pixelate`, `block_size=2..128`), and Sobel Edge Detection (`edge`, `roughness=0..4`).
- No external AI inference.
- No persistent image storage.

Measure actual CPU time, memory usage, and latency.

Do not assume local benchmarks accurately represent production performance.

## Security

- Validate file size and actual image format.
- Validate image dimensions before full decoding where possible.
- Reject malformed and oversized inputs.
- Avoid decompression bombs and unbounded allocations.
- Do not log uploaded image contents.
- Do not introduce secrets into source code.
- Do not introduce outbound network access without justification.

## Architecture

Separate the application into:

1. HTTP request handling
2. Input validation
3. Image decoding and resizing
4. Image filter implementations
5. Image encoding

Image filters should be implemented as independently testable functions.

## Initial Milestone

Implement only:

- A minimal Spin HTTP application.
- POST /api/transform
- Grayscale conversion
- JPEG output
- Input validation
- Unit tests
- Local build and execution instructions

Do not implement additional filters or the frontend until the initial milestone is validated.

## Testing

Run relevant tests after every implementation change.

Before marking a task complete:

1. Format Go code.
2. Run unit tests.
3. Run the Spin build.
4. Perform an HTTP smoke test.
5. Report any failures or unverified assumptions.

Never claim that a test passed unless it was actually executed.

## Agent Workflow

Before implementation:

1. Inspect the existing repository.
2. Read README.md and architecture documentation.
3. Identify the smallest change needed.
4. Explain any important architectural decisions.

During implementation:

1. Make focused changes.
2. Add tests for new behavior.
3. Avoid unrelated refactoring.
4. Update documentation when behavior changes.

After implementation:

1. Summarize the changes.
2. Report commands executed and results.
3. Identify remaining risks.
4. Propose the next small milestone.

## Deployment Rules

- Do not deploy without explicit user approval.
- Do not modify Akamai account resources without approval.
- Do not create paid cloud resources automatically.
- Do not commit credentials or tokens.

## Communication

- Communicate progress and explanations in Japanese.
- Write source code, comments, documentation, and commit messages in English.
- Be concise and technically precise.
- Clearly distinguish verified facts from assumptions.
