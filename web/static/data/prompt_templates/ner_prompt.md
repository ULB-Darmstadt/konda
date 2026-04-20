You are an expert in extracting structured knowledge from research texts and datasets in the domain of {{ .Domain }}. Given a summary of research papers, datasets, README files, and other metadata sources, your task is to extract relevant key concepts, domain-specific terms, and named entities that describe the dataset. The goal is to generate a structured list that will guide the knowledge graph creation process.

Your task is to:

1. Extract meaningful named entities and domain-specific terms (e.g., scientific terms, dataset-specific entities, processes, techniques, methodologies, technologies).
2. For each extracted name, infer three broader categories that describe the entity. These suggested categories/types should generalize the entity - especially if it is highly specific - by leveraging contextual clues, definitions, or relevant ontological terms.
3. Identify the most important entities that are key in describing the research effort.

Given these inputs:

1. Some additional remarks that the user did in regards to this task.
2. A JSON list of potential ontology terms that can be used for the suggested types
3. A summary of the research papers, readme files, etc.
4. A summary of the dataset in question.

Especially focus on the dataset file tree, dataset summary and entities as well as broader categories that semantically describe the dataset.
The most important entities are the ones from the dataset. 
There also needs to be enough entities from the context files so that the context of the dataset gets clear.
Don't use file names from the file tree as entities.

## Guidelines for Output

```json
[
    {"entity": "John Smith", "suggested_types": ["Author", "Researcher", "Person"] },
    { "entity": "Fine Blanking Process", "suggested_types": ["Metal Forming", "Manufacturing", "Process"] },
    { "entity": "Ground Truth Data", "suggested_types": ["Dataset", "Reference Data", "Validation Data"] },
    { "entity": "Force Measurement", "suggested_types": ["Experimental Variable", "Measurement", "Physical Quantity"] }
    { "entity": "TrainingData", "suggested_types": ["Dataset Component", "Machine Learning Data", "Preprocessed Data"] },
    { "entity": "GroundTruth", "suggested_types": ["Dataset Component", "Reference Data", "Validation Data"] },
    { "entity": "Metadata", "suggested_types": ["Dataset Description", "Data Documentation", "FAIR Metadata"] },
    { "entity": "Lubrication Coefficient", "suggested_types": ["Process Parameter", "Tribology", "Material Property"] },
    { "entity": "Hardness Test", "suggested_types": ["Experimental Procedure", "Material Testing", "Mechanical Property Assessment"] },
    { "entity": "RWTH Aachen University", "suggested_types": ["Organisation", "Academic Institution", "Research Center"] },
]
```

Only answer with the result in a structured json format like above, don't add any other explanations or information, just the json.

---

Here is the data that you need to analyze according to the above mentioned criteria:

- Additional Remarks from the user

```
{{ .Remarks }}
```

- Ontology Terms that might be useful

```json
{{ toJSON .OntologyTerms}}
```

- Summary of research paper, readme, etc.

```markdown
{{ .ContextSummary }}
```

- Dataset File Tree
The file structure of the dataset may contain relevant information about data organization and contextual clues:

```
{{ .FileTree }}
```

- Dataset Content  
The dataset content is provided in JSON format. Identify and categorize named entities, incorporating relevant terms from the pre-extracted entities while also detecting any new entities present in the data.

```json
{{ toJSON .FileContent }}
```

- Summary of Dataset

```markdown
{{ .DatasetSummary }}
```

Focus on exactly the top {{ .ExtractionCount }} most important entities that are important metadata that describe the files semantically.
