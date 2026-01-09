---
title: "12. NLP: Embeddings"
date: "2026-01-05"
description: "The Geometry of Meaning : Turning meaning into maths."
tags: ["NLP"]
pinned: false
draft: true
---

**Embeddings** are the mapping of discrete symbols into high-dimensional vector spaces. It is the translation of discrete, symbolic and combinatorial nature of human language (words, chars, syntax) into continuous mathematical substrate that computational models can manipulate.

> Embeddings are not merely static dictionaries; they are dynamic, geometric manifolds where the concepts of similarity, analogy, and context are encoded as distances, angles, and trajectories.

---

## 1. Theoretical Foundation

> **Goal** : Mapping discrete symbols (words) into continuous vector space $\mathbb{R}^d$ such that geometric proximity reflects semantic similarity.

### Vocabulary Gap

It is the fact that the words (strings) share no inherent relationship in their raw form (ASCII or Unicode Representation). To a computer (which operates on numerical data), the string *dog* and *animal* are as distinct as *dog* and *laptop*.

To bridge this vocabulary gap, 2 fundamental hypotheses from 2 distinct fields are used :

1. **The Distributional Hypothesis (Linguistics)**
2. **The Manifold Hypothesis (Geometry)**

### The Distributional Hypothesis (Linguistics)

The famous words of John Rupert Firt :

> You shall know a word by the company it keeps.

It can be stated mathematically as : the meaning of a word is defined by the probabilty distribution of other words $c$ (context) appearing in its vicinity.
- It is not an intrinsic property of the word $w$ itself.

$$ P(C|w) $$

- $w$ : target word
- $C$ : set of all possible context words.
- Probability of the context words given the target words.

This hypothesis is the bedrock of all embedding models.

---

### The Manifold Hypothesis (Geometry)

It states that, while the data might appear to exist in a space with thousands or millions of dimensions, the **meaningful data** actually sits only on a much simpler, lower dimensional shape called **manifold**, hidden inside that massive space.

---

#### Manifold

> A manifold is a topological space that is **locally Euclidean**.

- **Eucildean Space** : Standard flat space (eg. line, flat plane, 3D room) where standard geometry rules apply.
- **Locally** : It means in a small neighbourhood around any point.

- *1D Manifold* : A circle.
    - Zooming in on a small piece of circle, will appear like a straight line.
- *2D Manifold* : A donut shape.
    - Zooming in on the surface, it will look like a flat plane.

---

Thus, while the data technically has thousands of variables (dimensions), it doesn't actually fill up that space. Instead, the valid data points cling to a specific, curved shape floating inside that space.

A language may have a vocab of 100,000 words, so there will be a space of $\mathbb{R}^{100,000}$, but due to grammer, syntax and semantics not all points(words) will make sense. These rules force valid sentences to cluster together on a curved, continuous surface : manifold.

The goal of embedding models is to ignore the empty void of high-dimensional space and discover the shape of manifold and flattening it out.

---

### Early Attempts and Curse of Orthogonality

#### One-Hot Encoding

- The most basic mathematical representation of a vocab $V$ of size $|V|$.
- Each word is assinged a basis vector in $\mathbb{R}^{|V|}$.

For the $i^{th}$ word in the vocabulary, the vector $\mathbb{w}^i$ is defined as :

$$ \mathbb{w^i} = [0, 0, \dots, 1, \dots, 0]^\top $$

- $1$ is at the $i^{th}$ position.

<h4 class = "pink">Mathematical Failure</h4>

1. **Sparsity** : As the vocab size increase, the dimensionality explodes but the information content per vector remains exactly 1 bit.

2. **Orthogonality** : Dot product gives the similarity measure between 2 vectors. For any 2 distinct words $\mathbb{w}^i$ and $\mathbb{w}^j$ ($i \ne j$) :

$$ \mathbb{w}^i \cdot \mathbb{w}^j = \sum_{k=1}^{|V|} \mathbb{w}_k^i \mathbb{w}_k^i = 0 $$

- Because either or both of $\mathbb{w}^i$ and $\mathbb{w}^j$ will be 0 $\forall k$.

Also the Euclidean distance between 2 words will always be constant :

$$ \| \mathbb{w}^i - \mathbb{w}^j \|_2 = \sqrt{1^2 + 1^2} = \sqrt{2} $$

> Since there is no similarity (due to orthogonal vectors) and all the words being equidistant from all other words in the vector space, the geometry is broken and no meaningful inference can be drawn from the geometry of the space.
>- 'Hotel' and 'Motel' are at same distance as 'Hotel' and 'Water'.

---

#### Frequency Approach : TF-IDF

**Term Frequency-Inverse Document Frequency** uses continuous sparse vectors instead of binary vector.

For a term $t$ in a document $d$ :

$$ w_{t,d} = \text{tf}(t,d) \times \text{idf}(t) $$

- $\text{tf}(t,d) = \frac{\text{count}(t,d)}{\sum_{t' \in d}\text{count}(t',d)} $ 
    - Local Importance.
    - It measures how prominent is term $t$ within specific document $d$.
    - It is the count of the term $t$ in document $d$ normalized by count of all the terms in document $d$.

- $\text{idf}(t) = \log \frac{N}{|\{d \in D : t \in d\}|}$
    - Global Importance.
    - It measures how informative is term $t$ across the entire entire corpus.
    - $N$ : total no. of documents.
    - Denominator is the number of documents containing term $t$.
    - If a word is present in all documents, then ifd becomes ($\log(N/N) =) 0$.
        - Thus, cancels out the words weight.
    - If a word appears very less, then idf will be high, boosting the signal of the rare word.

>- TF says, "This word is frequent here." 
>- IDF says, "But is that rare generally?" 
>
>TF-IDF is the product of these two: High Frequency locally + High Rarity globally = High Importance.

It captures the importance of a word, but it still results in sparse vectors of size $|V|$. Thus, the geometric problems still remain.
- Synonyms don't occur together generally in the same document, so their vectors will remain nearly orthogonal.

---

### Latent Semantic Analysis (SVD)

One of the first approach to compress the sparse matrix into a dense one. It is like performing *Manifold Learning* via Linear Algebra.

[Singular Value Decomposition(SVD)](./Maths-SVD.md) is applied to a **Term-Document** $X$ of size $m\times n$, where $m$ is the vocab size and $n$ is the number of documents. The matrix is decomposed into :

$$ X = U\Sigma V^\top $$

To learn the embeddings, top $k$ sigular values are only kept.

$$ X = U_k\Sigma_k V_k^\top $$

SVD forces the data into smaller number of dimensions. Words that occur in similar document contexts (similar columns in $X$) will be squeezed into similar orientations in the reduced space $U_k$. 

---

### Pointwise Mutual Information (PMI)

Raw co-occurrence counts are biased by frequency. eg. the word `the` co-occurs with everything but that doesn't mean that it is semantically related to a word like `banana`.

> Thus, a measure is needed that compress **actual probability** to **expected probabiliy**.

For 2 words $x$ (word) and $y$ (context), PMI is :

$$ \text{PMI}(x,y) = \log_2{\frac{P(x,y)}{P(x)P(y)}} $$

- $P(x,y)$ : probability of words 11$x$ and 12$y$ appearing together (joint probability).
- $P(x)P(y)$ : probability of them appearing together if they were statistically independent.

Interpretation of PMI :

- PMI $> 0$: $x$ and $y$ co-occur more often than chance (Semantic association).
- PMI $= 0$: $x$ and $y$ are independent.
- PMI $< 0$: $x$ and $y$ co-occur less often than chance (Complementary distribution).

#### Positive Pointwise Mutual Information (PPMI)

$log(0) = - \infty$ and negative associations are hard to model sparsely, thus positive PMI is used :

$$ \text{PPMI}(x,y) = \max{(\text{PMI}(x,y),0)} $$

---

To summarize till now, 
- Raw text lacks numerical meaning. 
- One-Hot encoding fails geometrically (orthogonality). 
- SVD offers a linear algebra solution to compress the space and find latent meaning, but it is computationally expensive ($O(mn^2)$) and hard to update with new data. 
- PMI gives a rigorous statistical target for similarity.

---

## 2. Neural Shift (Static Embeddings)

### Word2Vec Family

Introduced in the paper [Efficient Estimation of Word Representations in Vector Space](https://arxiv.org/abs/1301.3781) by Thomas Mikolov, et.al., it were architectures of 2 models **CBOW** & **Skip-gram** and in the paper [Distributed Representations of Words and Phrases and their Compositionality](https://arxiv.org/abs/1310.4546) their optimization was introduced. It allowed computers to treat or understand words not as strings but as points in a complex, multi-dimensional semantic space.

Word2Vec aimed to create **Dense Distributed Representations** (Embedding) :
- **Dense** : Short vectors (smaller dimensions, eg. 300 dimension) full of non-zero real numbers.
- **Distributed** : Meaning of the words are *smeared* across all the 300 dimensions.

> 



