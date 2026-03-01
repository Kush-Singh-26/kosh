---
title: "12. NLP: Embeddings"
date: "2026-01-05"
description: "The Geometry of Meaning : Turning meaning into maths."
tags: ["NLP"]
pinned: false
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

- $ 1 $ is at the $i^{th}$ position.

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

- PMI $ \gt 0 $ : $ x $ and $ y $ co-occur more often than chance (Semantic association).
- PMI $ = 0 $ : $ x $ and $ y $ are independent.
- PMI $ \lt 0 $ : $ x $ and $ y $ co-occur less often than chance (Complementary distribution).

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

## 2. Neural Shift : Static Embeddings

### Word2Vec Family

Introduced in the paper [Efficient Estimation of Word Representations in Vector Space](https://arxiv.org/abs/1301.3781) by Thomas Mikolov, et.al., it were architectures of 2 models **CBOW** & **Skip-gram** and in the paper [Distributed Representations of Words and Phrases and their Compositionality](https://arxiv.org/abs/1310.4546) their optimization was introduced. It allowed computers to treat or understand words not as strings but as points in a complex, multi-dimensional semantic space.

Word2Vec aimed to create **Dense Distributed Representations** (Embedding) :
- **Dense** : Short vectors (smaller dimensions, eg. 300 dimension) full of non-zero real numbers.
- **Distributed** : Meaning of the words are *smeared* across all the 300 dimensions.

> Its objective is to have the words appering in similar context to have similar vectors too.

Both of these architectures use shallow networks, i.e., they have only 1 hidden layer. And this hidden layer has **no non-linear actiavtion function**. Hence, it is also called *Projection Layer*. It is effectively just a lookup table. The inputs are **one-hot vector**, at lets say the index `5` of vector $x$ is 1. Then multiplying $x$ with weight matrix $W$, will simply select the 5th row of the matrix. It speeds up the training.

#### A. Continuous Bag-of-Words (CBOW)

> Predict the **target word** based on the context words (surrounding words) $w_{t-c}, \dots, w_{t+c}$.

- **Input** : Context words within a window (eg. 3 words before and 3 words after).
- Process :
    1. One-hot vectors of the context words are projected via a weight matrix $W$ (**embedding matrix**).
    2. These vectors are averaged.
    3. The averaged vector is projected to the output layer.

>- **Pros** : Faster to train. Better accuracy for frequent words.
>- **Cons** : Averaging context loses specific word order and information (**smears** the context).

#### B. Skip-Gram

> Predict the **context words** $w_{t-c}, \dots, w_{t+c}$ based on the target word.

- **Input** : A single *target* word.
- Process :
    1. Model uses the target word to predict the probability distribution of words likely to appear in its window (before and after).
    2. For a given target word, pairs of `(target, context)` are treated as training samples.

>- **Pros** : Works well with small amounts of training data; represents rare words and phrases well.
>- **Cons** : Slower to train (more training pairs created per window).

#### Objective Function (Skip Gram)

Maximize the average log probability of context words given center words across the entire corpus of size $T$.

$$ J(\theta) = \frac{1}{T} \sum_{t=1}^T \sum_{-c\le j\le c, j\ne 0} \log{P(w_{t+j}|w_t)} $$

- Standard *Softmax* defines this probability as :

$$P(w_O | w_I) = \frac{\exp(\mathbf{v}'_{w_O}{}^\top \mathbf{v}_{w_I})}{\sum_{w=1}^{|V|} \exp(\mathbf{v}'_w{}^\top \mathbf{v}_{w_I})}$$

- $\mathbf{v}_{w_I}$: Vector representation of the input (center) word.
- $\mathbf{v}'_{w_O}$: Vector representation of the output (context) word.

> **The Bottleneck** : The denominator requires summing over the entire vocabulary ( $ 100,000+ $ words) for every single training example. This is computationally infeasible.

---

#### Optimization

##### A. Hierarchical Softmax

Instead of a flat array, the vocabulary is arranged in a **Huffman Tree** (frequent words are present closer to the root). The probability of a word $w$ is the product of probabilities of the decisions of turning left or right while traversing the tree from root to $w$.
- Complexity reduces from $O(|V|)$ to $O(\log_2 |V|)$.
- This uses sigmoid functions at branch points rather than one giant softmax.

##### B. Negative Sampling (Standard Approach)

Instead of calculating the probability over the whole vocabulary, the problem is reframed as a **Binary Classification Task**. So the corpus is divided into :
- **Positive Examples**
    - $(w,c)$ (real data) 
- **Negative Examples**
    - $(w,w_i)$, $w_i \sim P_n(w)$ (fake data)
    - Noise pairs constructed by pairing $w$ with randomly sampled words that are not true in its context. 

The loss function is :

$$J = \log \sigma({v'_{w_O}}^\top v_{w_I}) + \sum_{i=1}^{k} \mathbb{E}_{w_i \sim P_n(w)} [\log \sigma(-{v'_{w_i}}^\top v_{w_I})]$$

- $v_{w_I}$ : **Input Embedding** of the center word $w$.
- ${v'_{w_O}}$ : **Output embedding** of the context word $c$.

- $\log \sigma({v'_{w_O}}^\top v_{w_I})$
    - ${v'_{w_O}}^\top v_{w_I}$ : this dot product measures similarity between word and context
    - So, the sigmoid gives the **probability that $(w,c)$** is a true pair.
    - log is used for numerix stability and easier optimization.
    - So, it encourages ${v'_{w_O}}$ $v_{w_I}$ to have a large dot product.
    - Makes the model confident that ($w,c$) is a real pair.

- $\sum_{i=1}^{k} \mathbb{E}_{w_i \sim P_n(w)} [\log \sigma(-{v'_{w_i}}^\top v_{w_I})]$
    - Because, $\sigma(-x) = 1 - \sigma(x)$, it maximizes probability that the pair ($w,c$)is not real.
    - $\mathbb{E}_{w_i \sim P_n(w)}[\cdot]$
        - The average value over words sampled from $P_n$.

> $P_n{w} \propto U(w)^{0.75} $ is the noise distribution from which $w_i$ is drawn randomly.

---

This paper discovery that the learned vector space preserved linear semantic relationships.

$$\text{vector(King)} - \text{vector(Man)} + \text{vector(Woman)} \approx \text{vector(Queen)}$$

---

### GloVe (Global Vector)

So till now 2 dominant approaches to word representation have been presented :

- **Matrix Factorization**
    - eg. [LSA](#latent-semantic-analysis-svd)
    - These methods rely on Global Statistics. 
    - They look at the entire corpus at once and decompose a massive document-term matrix
    - > Efficiently captures global statistical information (frequency) but are computationally expensive and often fail to capture local semantic nuances.

- **Local Context Window**
    - eg. [Word2Vec](#word2vec-family)
    - These methods learn by sliding a small widow over the text.
    - > They capture complex linguistic patterns and analogies well but fail to explicitly utilize global co-occurence count of the corpus.

So, GloVe model was created which used the best of both worlds.

> Create a model that uses **Global** matrix factorization methods to optimize a loss function derived from **Local** context window observations.

---

GloVe paper showed that while the raw probabilities ($P_{ik}$) are noisy, but the **ratio of proababilities** is *precise*.

- Let $i = \text{ice}$
- Let $j = \text{steam}$

Analyzing the relationship of these words with various *probe* words :

|Probe Word ($k$)|$P(k∣\text{ice})$|$P(k∣\text{steam})$|Ratio $\frac{P_{jk}}{​P_{ik}}​​$|Interpretation|
|---|---|---|---|---|
|solid|High (Ice is solid)|Low (Steam is not)|Large (>1)|The ratio distinguishes ice from steam.|
|gas|Low (Ice is not gas)|High (Steam is gas)|Small (<1)|The ratio distinguishes steam from ice.|
|water|High (Ice is water)|High (Steam is water)|$\approx$1|The word is relevant to both, so it doesn't help distinguish them.|
|fashion|Low (Irrelevant)|Low (Irrelevant)|$\approx$|The word is relevant to neither, so it doesn't help distinguish them.|

This proves that the meaninig is embedded in the ratio. Very large or small ratio means the probe word $k$ is discriminative. If the word is unrelated or equally related to both the ratio is closer to 1.

---

#### Deriving the GloVe model

So a function is needed which gives ratio of the probabilities, given the vector of the words. In the vector space, the meaning of the words is encoded as the offset between words as observed in Word2Vec. Therefore, the relationship between words should use the difference betweeen their vectors :

$$ F(\mathbf{w}_i - \mathbf{w}_j, \tilde{\mathbf{w}_k}) = \frac{P_{ik}}{P_{jk}} $$

Convert the vectors in left side to scalar using dot product.

$$ F((\mathbf{w}_i - \mathbf{w}_j)^\top \tilde{\mathbf{w}_k}) = \frac{P_{ik}}{P_{jk}} $$

Now the subtraction needs to be converted to division. Assuming the function $F$ is an exponential fucntion (so, $e^{A-B} = e^A / e^B$), taking log on both sides will give a logic connection or bridge between both sides of equation.

$$ (\mathbf{w}_i - \mathbf{w}_j)^\top \tilde{\mathbf{w}_k} = \log(\frac{P_{ik}}{P_{jk}}) $$

Using the log rules $\log(a/b) = \log(a) - \log(b)$ :

$$ \mathbf{w}_i^\top \tilde{\mathbf{w}_k} - \mathbf{w}_j^\top \tilde{\mathbf{w}_k} = \log(P_{ik}) - \log(P_{jk}) $$

Looking at matching terms on both sides, we get :

$$ \mathbf{w}_i^\top \tilde{\mathbf{w}_k}  = \log(P_{ik}) $$

---

The probability $P_{ik} = P(i|k)$ is conditional probability. It is equal to $P_{ik} = \frac{X_{ik}}{X_i} $. 
- $X_{ik}$ : co-occurence count of $i$ and $k$.
- $X_i$ : total count of word $i$.

Thus,

$$\mathbf{w}_i^\top \tilde{\mathbf{w}}_k = \log(\frac{X_{ik}}{X_i}) = \log(X_{ik}) - \log(X_i)$$

$$\mathbf{w}_i^\top \tilde{\mathbf{w}}_k + \log(X_i) = \log(X_{ik})$$

This equation is not symmetric. In a co-occurence matrix, the relationship is symmetric. The number of times *ice* appears with *steam* is same as *steam* appearing with *ice* ($X_{ik}=X_{ki}$). Thus, in the eqaution if $i$ and $k$ are swapped, it should look the same.

But because $log(X_i)$ depends only on $i$, there is a need for a term representing $k$. Thus, bias terms are added to restore symmetry for the other word. 
> $k$ is replaced with $j$ in the final notation.

$$ \mathbf{w}_i^\top \mathbf{w}_j + b_i + b_j = \log(X_{ij}) $$

> Mathematically, $\log(X_{ij})$ depends on the frequency of word $i$ and word $j$ individually. The bias terms absorb these independent frequencies so that the dot product $\mathbf{w}_i^\top \mathbf{w}_j$ only has to capture the interaction between the words, not their raw popularity.

---

#### Loss Funtion of GloVe

The goal is to minimize the difference between the learned relation (LHS) and the actual values or statistics (RHS). Weighted Least Square is used :

$$J = \sum_{i,j=1}^V f(X_{ij}) (\underbrace{\mathbf{w}_i^T \mathbf{w}_j + b_i + b_j}_{\text{Prediction}} - \underbrace{\log X_{ij}}_{\text{Target}})^2$$

##### The weighting factor in GloVe

- Stopwords like ("the", "and", "is") can co-occur with almost everything millions of times. So their domination on the loss funtion must be controlled.
- Many pairs might appear just 1-2 times. So the contribution of these noisy pairs must be controlled.

So, the weighting factor ($f(X_{ij})$) is used :

- If $X_{ij} = 0$, $f(0) = 0$ (The pair is ignored; avoids $\log 0$ error).
- It increases as co-occurrence count increases (trust frequent data more).
- It caps at a maximum value (usually 1.0) so that extremely frequent words don't dominate.

---

So to summarize, GloVe explicitly factorizes the logarithm of the co-occurence matrix. It ensures that the **dot product of 2 words equals the log of their probabilities of co-occurence**. It captures the global corpus statistics in a linear vector space.

---

### FastText (Sub-Word Information)

#### Issues with GloVe and Word2Vec

- **Atomic Unit** : In Word2Vec the vectors for `Apple` and `Apples` are completely independent of each other. It can't infer that they share a root meaning just by looking at their spelling.
- **OOV Problem** : If the model sees a word it didn't encounter in traning data, it would assign it a generic `<UNK>` token to it, loosing all its meaning.
- **Morphologically Rich Language** : Languages like German, Turkish, or Finnish use heavy compounding (combining words). So, a single root word might have hundreds of variations. Storing a unique vector for every single variation is computationally inefficient and sparse.

---

> **FastText** proposes that a word is actually a **bag if character n-grams**.

#### N-grams

It is simply a sliding window of $n$ characters. Also special boundary chars `<` and `>` are added at beginning and end respectively of a word.

Eg. `Apple` with $n=3$ (trigrams) will split into : `<ap` (prefix), `app`, `ppl`, `ple` and `le>` (suffix).

Along with these trigrams, the original word `apple` is also included to capture the specific meaning.

Instead of learning one vector for `apple`, the model learns a vector for **every unique n-gram** in the vocab.

The final representation for a word $\mathbf{w}$ is the sum (or average) of all its n-gram vectors $\mathbf{z}_g$ :

$$ \mathbf{w} = \sum_{g\in G_w} \mathbf{z}_g $$

- $G_w$ is the set of n-grams appearing in word $w$.
- $\mathbf{z}_g$ is the learned vector for a specific n-gram $g$.

The scoring function tells that the similarity between a target word and a context word is the sum of the similarities between all the target's parts (n-grams) and the context word.

$$ s(w,c) = \sum_{g\in G_w} \mathbf{z}_g ^\top \mathbf{v}_c $$

---

Thus, this models handles the OOV Problem and Morphologicaly Rich Language issues by constructing the vector for the words by summing the vectors of its n-grams.

---

To summarize till now :

- **Word2Vec** stripped the hidden layer to make it efficient, introducing Negative Sampling to approximate the Softmax.

- **GloVe** combined local context windows with global count statistics.

- **FastText** broke the atomic word assumption to handle morphology.

---

## 3. The Modern Pre-Processing : Tokenization

This next phase addresses the bridge between human language (strings) and machine language (integers). 

In Word2Vec, the atomic unit was the word itself which led to various problems like OOV, Explosion of Vocabulary & Morphological blindness.

> Thus, instead of mapping whole words, sub-words are mapped.

Tokenizers are already covered in [this post](./NLP-Tokenization.md).

### How `nn.Embedding` works.

The next step after creating the tokens (`["The", "##mbed", "##ding"]`) is to convert them to Integers (ID) using the vocabulary map `[101, 3923, 8321]`.

The `nn.Embedding` layer is just a **look-up table**. Let $V$ be the vocabulary size and $d$ be the embedding dimension. Thus, the Embedding layer is a learnable matrix $E \in \mathbb{R}^{|V|\times d}$. If the input is a token ID $k$ (an integer), then it is like multiplying an one-hot vector $\mathbf{x}_k$ is 1 in position $k$ and 0 elsewhere with matrix $E$.

$$\mathbf{v} = \mathbf{x}_k^T E$$

> Since $\mathbf{x}_k$ is all zeros except at index $k$, this matrix multiplication simplifies to just selecting the $k$-th row of $E$. It directly indexes the array and thus avoids multiplication.

If a layer is intialized like : `nn.Embedding(num_embeddings=10000, embedding_dim=300)`, it would typically use Normal Distribution ($\mathcal{N}(0, 1)$) to initialize :

$$E_{initial} = \begin{bmatrix} 0.01 & -0.42 & \dots \\ -1.2 & 0.05 & \dots \\ \vdots & \vdots & \ddots \end{bmatrix}$$

A **dense matrix** is initialized but the gradients (updates) for this embedding matrix will be sparse. If the vocabulary will have 50,000 words and batch size is 64 and the max sentence length is 10, then total tokens in one batch $ 64 \times 10 = 640 $ tokens. So when error is backpropagated, the gradient $\nabla E$ (which tells how much should the embedding matrix change) is calculated. Only 640 rows will be active out of the 50,000 rows. 49,360 rows gradient will be exactly 0. Thus, the gradient matrix for this batch will be **98.7% sparse**.

---

## 4. Contextual Embeddings : Transformer Era

### Failure of Static Embeddings

A word can have multiple meanings depending on the context it is being used in.
- Sentence A : "I sat on the **bank** of the river."
- Sentence B : "I went to **bank** to withdraw money."

In static embedding models, like Word2Vec, $\mathbf{v}_{bank}$ is a single point in space. Mathematically, this vector is the weighted average of all its contexts. This vector will end up somewhere between money and water, not satisfying any of the context properly. 

> It effectively **smears** the meaning.

Thus, a function $f$ is needed such that :

$$ f(\text{bank}, \text{Context}_A) \ne f(\text{bank}, \text{Context}_B) $$

This is where the **transformer** architecture comes in. In a modern LLM, this is the most basic sequence of tasks performed on an input sequence :
1. *Tokenize* : `["The", "bank", "is", "open"]` $\to$ `[101, 2943, 831, 442]`
2. *Static Lookup* :  Fetch static vectors from nn.Embedding. Let this sequence be $X$.
    - $X = [\mathbf{x}_1, \mathbf{x}_2, \mathbf{x}_3, \mathbf{x}_4]$
3. *Contextualization (The Transformer Layers)* : When these vectors pass through self-attention layers, the information from `open` flows into `bank`.
4. *Output* : A new sequence of vectors $Z$ is received.
    - $Z = [\mathbf{z}_1, \mathbf{z}_2, \mathbf{z}_3, \mathbf{z}_4]$

> Here, $\mathbf{z}_2$ is a "contextualized embedding." It is no longer just `bank`; it is **bank-associated-with-opening.**

---

### How Self-Attention physically changes the vectors

As discussed in the [Transformers Post](./NLP-transformer.md), every input vector $\mathbf{x}_i$ is projected into three distinct subspaces using learnable weight matrices $W_Q, W_K, W_V$:
- **Query** : ($\mathbf{q}_i = \mathbf{x}_i W_Q$)
    - What this token is looking for.
- **Key** : ($\mathbf{k}_i = \mathbf{x}_i W_K$)
    - What this token contains (content/identity)
- **Value** : ($\mathbf{v}_i = \mathbf{x}_i W_V$)
    - The actual information this token will pass along.

Let `bank` be the current token. To know which definition to use it will :
1. **Query** : `bank` broadcasts a query: "Are there any words here related to water or finance?"
2. **Keys** : 
    - `The` says: "I am a determiner." (Low match)
    - `River` says: "I am a body of water." (High match!)
    - `Deposit` says: "I am a financial action." (Low match in Sentence A, High in B).
3. **Score** : Calculate the dot product between query of `bank` and keys of every other word.

$$ \text{Score}_{i,j} = \mathbf{q}_i \cdot \mathbf{k}_j $$

Then these scores are normalized using Softmax to get Attention Weights ($\alpha$) which will sum up to 1.

$$\alpha_{i,j} = \text{Softmax}\left(\frac{\mathbf{q}_i \cdot \mathbf{k}_j^T}{\sqrt{d_k}}\right)$$

- $\alpha_{\text{bank, river}} \approx 0.8$  (High Attention)
- Other 2 will be $\approx 0.1$ (Low Attention)

The new vector for `bank` ($\mathbf{z}_{\text{bank}}$) is the weighted sum of the Values of all context words : 

$$\mathbf{z}_{\text{bank}} = 0.8 \mathbf{v}_{\text{river}} + 0.1 \mathbf{v}_{\text{the}} + 0.1 \mathbf{v}_{\text{bank}}$$

> The vector for `bank` has physically moved in vector space. It has been pulled towards the `river` vector. It is now a **river-bank** vector.

#### During Training

When initialized, `nn.Embedding` is a matrix of size `[Vocabulary Size x Dimension]` filled with random noise.
- As the model learns or reads training data, it will come across a word like `bank` many times each appearing in a different context.
    - If the context is "fish in the bank", the error signal will pull the `bank` vector towards *nature*.
    - If the context is "deposit in the bank", the error signal will pull the `bank` vector towards *finance*.

> By the end of training, the vector of `bank` will settle in the **mathematical center** of all its meanings.

Once the training is done, the matrix is **frozen**. It becomes a read-only lookup table.

#### During Inference

During inference, irrespective of the input sequence ("The bank of the river" or "The bank of America"), the same exact prototype vector for the `bank` is retrieved.

Now based on the context, the self-attention mechanism will create a new vector for the word `bank` which will be added to the original vector of `bank` as **residual connections** are used in transformers.

---

### Positional Encoding

Dot product is permutation invariant. $(a\cdot b) = (b \cdot a)$. This means the set `{"man", "bites", "dog"}` produces the exact same attention scores as `{"dog", "bites", "man"}`. The model has no concept of order.

So, some position information is injected before the first Transformer layer :

$$X_{\text{input}} = \text{Embedding}(X) + \text{PositionalEncoding}(Position)$$

These embeddings can be absolute like in original transformer or rotational/relative like in modern models like LLaMA models.

---

### Embeddings in RAGs

**Retrieval-Augmented Generation** (RAG) is an AI framework that boosts Large Language Models (LLMs) by connecting them to external, up-to-date knowledge bases, allowing them to retrieve relevant information before generating an answer, making responses more accurate, factual, and context-aware, without needing costly model retraining.  

In RAGs embeddings are the engine that powers the **retrieval step**. In RAG instead of caring about individual words like `bank`, we start calculating vectors for the *entire sentence or paragraphs*. To seach in a massive library of documents and the exact words used in the documents are unknown, then RAGs are useful as they provide **semantic search**.

#### Embedding Models

Instead of using a decoder only transformer, embedding models typically consist of **Encoder-only** models. These models can be **BERT / Sentence BERT** or OpenAI's **text-embedding-3**, etc. It uses Bi-directional Attention. So token for `bank` can see both `river` (future) and `bank` (past) simultaneously to understand the *entire* sentence structure at once.


So is a 10-word sentence is fed into the transformer, output of 10 vectors is received. So a **Pooling Layer** is added to get 1 vector which replaces the prediction head of decoder-only transformers.

There are 2 mains strategies for pooling :

##### A. `[CLS]` Token Strategy

This is like [BERT Models](./NLP-BERT.md) where a special token `[CLS]` (Classification) is prepended to start of every sentence.

After going through the self-attention layers of the transformer, `[CLS]` will have absorbed the entire context of the sentence and this token will be treated as the representation of the whole sentence.

##### B. Mean Pooling Strategy

We take the output vectors for all tokens and calculate their mathematical average.

$$\mathbf{V}_{\text{sentence}} = \frac{1}{N} \sum_{i=1}^{N} \mathbf{z}_i$$

This is often more effective than strategy A.

---

The goal of embedding models is to **organize the vector space**.
- **Anchor** : "The dog is happy."
- **Positive** : "The puppy is joyful." (We want this close).
- **Negative** : "The cat is sleeping." (We want this far away).

The model will adjust its weights till :

$$\text{Sim}(\text{Anchor}, \text{Positive}) \gg \text{Sim}(\text{Anchor}, \text{Negative})$$

---

All documents are initially processed, sentence vectors are produced and stored in *Vector Database*. When a user wants to retrieve some text, the user's query is vectorised and compared with all the stored vectors in the vector database using metrics like **Cosine Similarity** and the mathematically most closest document to the query is returned.   

---

With this journey about the embeddings from theoretical foundations to staic embeddings, tokenization and how `nn.Embeddings` work and finally encountering contextual embeddings and how they are used by RAGs is completed.