---
title: "8. NLP: BERT"
date: 2025-07-25
description: "A guide on BERT models and how their architecture."
tags: ["NLP", "Transformers"]
pinned: false
---

## Bidirectional Encoder Representation from Transformer


Introduced in the paper [BERT: Pre-training of Deep Bidirectional Transformers for Language Understanding](https://arxiv.org/pdf/1810.04805), BERT is a language representation model. Earlier language models could only read text in one direction—either left-to-right or right-to-left. It's training consists of 2 phases, **Pre-Training** & **Fine-Tuning**. During the first phase the model pre-trains **deep bidirectional representations** from unlabbeled text by **jointly conditioning on both left and right context in all layers**. This allows to create great models for wide range of NLP tasks by just finetuning with a additional output layer.

## Architecture

- BERT is based on the [encoder](https://kush-singh-26.github.io/blogs/2025/06/22/Transformer.html#encoder) stack of the Transformer model.
- Unlike, the decoder of the transformer which is autoregressive and uses a mask to prevent future-peeking, enocoder processes the entire input sequence in a single pass.
- Self-Attention is designed to relate every token to every other token in the input sequence.
- This is one of the reason why the architecture is bi-directional.
- 2 models were trained in the paper :
    - **${BERT_{BASE}}$** with 12 transformer blocks each having hidden size = 768 and 12 Attention Heads.
    - **${BERT_{LARGE}}$** with 24 transformer blocks each having hidden size = 1024 and 16 Attention Heads.

> Here, transformer and encoder of the transformer are used interchangeably.

# Pre-Training

- The model is first pretrained to find intricate patterns of the language which creates powerful representations.
- It is done using 2 tasks **Masked Language Model** & **Next Sentence Prediction**.

## Masked Language Model (MLM)

- Unlike other language models whose task is to predict the next word in a sequence, MLM's objective is to **predict the original identity of randomly masked tokens based on the surrounding unmasked context**.

- First, the input is tokenized using [WordPiece](https://huggingface.co/learn/llm-course/en/chapter6/6).
- In each input sequence, 15% of the tokens are selected at random.
- Out of these selected tokens, :
    - 80% are replaced with a `[MASK]` token.
    - 10% are replaces with a random word.
    - 10% are left unchanged.

- This split allows the model to not just predict the blank but also be robust to corrupted input and maintain a high-quality representation for every single input token.

> This teaches the model to understand relationships **within** a sequence.

## Next Sentence Prediction (NSP)

> This teaches the model to understand relationships **between** sentences.

- It is done for important downstream tasks like Question-Answering.

- It is a simple binary classification problem.
- A pair of sentences (`A` & `B`) are given and the model must predict whether `B` is the actual sentence that follows `A` in the original text.

- 50% of the the pairs are positive, i.e., `B` `IsNext` of `A`.
- Rest 50% are negative pairs  `B` is `NotNext` of `A`.
    - In this case `B` is a random sentence drawn from the same corpus.

---

- `[CLS]`
    - It is a special token which is always placed at beginnin of the sentence.
    - It learns the representation from both the sentences and the output from its position is used to predict whether the sentence `B` comes next or not.
    - It is passed through a classification layer which outputs the probability of `IsNext` or `NotNext` class.

>  During fine-tuning, `[CLS]`'s final hidden state is often used as the input to a classifier for sentence-level tasks like sentiment analysis.

- `[SEP]`
    - It is also a special token which acts as separator between the sentences `A` and `B`.

---

## Loss Calculation

- The model is trained to optimize both the MLM & NSP objectives simultaneously.

- `MLM Loss`
    - cross entropy loss calculated over the predicted masked tokens.

- `NSP Loss`
    - binary cross entropy loss from classification of sentence pair.

> Thus, **Total Loss = MLM Loss + NSP Loss**.

---

## Input to the Model

- After tokenization, the embeddings for each token are passed to the model for each token.
- It is constructed by summing up 3 distinct embedding vectors.

1. **Token Embedding** :
    - Fundamental embedings for each token from in BERT's vocab.

2. **Segment Embedding** :
    - If the input consists of a pair of sentences, it is used to distinguish between them.
        - $E_A$ : **added to every token** of first sentence.
        - $E_B$ : **added to every token** of second sentence.
    
    - If the input consists of only a single sentence only $E_A$ embedding is added.

3. **Position Encoding** :
    - It is a unique vector which is added to each token based on its position in the sequence for positions 0 to 511 (which is the model's max-seq len).

---

The pre-training phase is a very compute and data intensive task. Thus, pre-trained BERT models are finetuned for the required down-stream task.

---

# Fine-Tuning

- The pre-trained model can now be quickly and efficiently adapted for a wide variety of down stream (specific) NLP tasks.

### 1. Add a Layer :

- Take the pre-trained BERT model and add a small, task-specific output layer on top.
- Examples :
    - simple classification layer for sentiment analysis
    - two layers to predict start/end tokens for question answering.

### 2. Initialize : 

- BERT part of the model is initialized with the pre-training weights.
- New layer output is initialized randomly.

### 3. Train on Labelled Data (Supervised Learning) :

- Model is then trained on a much smaller, task-specific labeled dataset.