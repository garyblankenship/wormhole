# 📚 Wormhole Documentation

Welcome to the comprehensive documentation for Wormhole, the ultra-fast Go LLM SDK. This directory contains detailed guides, technical specifications, and resources for developers using Wormhole.

## 🚀 Getting Started

Start here if you're new to Wormhole or need quick implementation guidance.

### [📖 quick-start.md](./quick-start.md)
**Get up and running in 2 minutes**
- Installation instructions
- Basic usage examples
- Essential patterns for immediate productivity
- Zero-to-hero implementation guide

### [🔧 advanced-features.md](./advanced-features.md)
**Enterprise-grade features and advanced patterns**
- Complex middleware configurations
- Performance optimization techniques
- Production deployment patterns
- Advanced streaming and concurrent usage

### [🌐 provider-guide.md](./provider-guide.md)
**Complete provider ecosystem guide**
- Available LLM providers (OpenAI, Anthropic, Gemini, Groq, Mistral, Ollama)
- Provider-specific capabilities and limitations
- Configuration examples for each provider
- Multi-provider setup patterns
- OpenAI-compatible provider consolidation architecture

### [🔄 provider-consolidation-migration.md](./provider-consolidation-migration.md)
**Provider Architecture Consolidation Guide (v1.4.0+)**
- Zero-breaking-change consolidation of OpenAI-compatible providers
- Architectural improvements and code reduction
- Migration guide and benefits overview
- Future provider addition patterns

## 🔧 Technical Deep Dives

Detailed technical documentation for advanced users and contributors.

### [⚡ performance-benchmarks.md](./performance-benchmarks.md)
**Benchmark data and performance analysis**
- 67ns core latency measurements
- Memory usage and allocation patterns
- Competitive analysis vs industry alternatives
- Performance optimization recommendations

### [🔄 json-utilities.md](./json-utilities.md)
**Robust JSON parsing for AI model responses**
- Handling Claude model regex patterns and escaped strings
- LenientUnmarshal() and specialized parsing utilities
- Real-world AI response parsing challenges
- Production-ready error handling patterns

### [🎯 openrouter-claude-guide.md](./openrouter-claude-guide.md)
**Structured output with Claude via OpenRouter**
- OpenRouter native vs Wormhole structured output approaches
- Complete implementation examples
- Performance and consistency comparisons
- Production usage patterns and best practices

### [🏗️ architecture-design.md](./architecture-design.md)
**Provider-aware model validation architecture**
- Dynamic model catalog support
- OpenRouter integration challenges and solutions
- Architectural design decisions and rationale
- Future extensibility considerations

## 📋 Development & Process

Resources for contributors and maintainers.

### [🤝 contributing.md](./contributing.md)
**Contribution guidelines and development workflow**
- Code of conduct and community standards
- Development setup and testing procedures
- Pull request process and review standards
- Release management and versioning

### [📝 changelog.md](./changelog.md)
**Complete release history and breaking changes**
- Version-by-version feature additions
- Breaking changes and migration requirements
- Bug fixes and performance improvements
- Detailed release notes for each version

### [✨ developer-experience.md](./developer-experience.md)
**Developer experience enhancements based on real feedback**
- Problems identified from production usage (Meesix integration)
- Solutions implemented to improve developer productivity
- Before/after comparisons with code examples
- Validated improvements and their impact

## 🔄 Migration & Examples

Practical guides for upgrading and understanding improvements.

### [🚀 migration-guide.md](./migration-guide.md)
**Breaking changes migration guide for v1.0+**
- API changes from mutable to immutable patterns
- Step-by-step migration examples
- Common pitfalls and solutions
- Compatibility considerations

### [📊 improvement-examples.md](./improvement-examples.md)
**v1.3.1 improvement demonstrations**
- Real-world usage scenarios and improvements
- Performance comparisons and metrics
- Code complexity reductions
- Developer productivity enhancements

### [🧠 memory-system-case-study.md](./memory-system-case-study.md)
**AI development partner implementation case study**
- Revolutionary memory management system architecture
- Claude Code integration patterns and hooks
- Persistent learning development partner capabilities
- Implementation details and lessons learned

---

## 📖 Documentation Categories

### **Essential Reading** 🎯
For immediate productivity: `quick-start.md` → `provider-guide.md` → `advanced-features.md`

### **Performance & Optimization** ⚡
Understanding speed and efficiency: `performance-benchmarks.md` → `json-utilities.md`

### **Integration Guides** 🔌
Specific use cases: `openrouter-claude-guide.md` → `migration-guide.md`

### **Development Resources** 🛠️
Contributing and maintaining: `contributing.md` → `changelog.md` → `developer-experience.md`

### **Case Studies & Examples** 📈
Learning from real implementations: `improvement-examples.md` → `memory-system-case-study.md`

---

## 🔍 Finding What You Need

- **New to Wormhole?** Start with `quick-start.md`
- **Performance questions?** Check `performance-benchmarks.md`
- **Provider issues?** Reference `provider-guide.md`
- **Upgrading versions?** See `migration-guide.md` and `changelog.md`
- **Contributing code?** Read `contributing.md`
- **OpenRouter + Claude?** Use `openrouter-claude-guide.md`
- **JSON parsing problems?** Reference `json-utilities.md`
- **Advanced patterns?** Explore `advanced-features.md`

## 🎯 Quality Standards

All documentation in this directory maintains:
- **Accuracy**: Tested examples and verified information
- **Completeness**: Comprehensive coverage of features and use cases
- **Clarity**: Clear explanations with practical examples
- **Currency**: Regular updates to reflect latest features and best practices

---

*Last updated: 2025-08-17 | Wormhole v1.3.1+*